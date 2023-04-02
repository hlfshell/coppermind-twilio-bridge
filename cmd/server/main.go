package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/twilio/twilio-go"
	api "github.com/twilio/twilio-go/rest/api/v2010"
	"github.com/urfave/cli/v2"
)

var client *twilio.RestClient

var numbers map[string]string

type Message struct {
	ID           string    `json:"id,omitempty"`
	Conversation string    `json:"conversation,omitempty" db:"conversation"`
	User         string    `json:"user,omitempty" db:"user"`
	Agent        string    `json:"agent,omitempty" db:"agent"`
	Content      string    `json:"content,omitempty" db:"content"`
	Tone         string    `json:"tone,omitempty" db:"tone"`
	CreatedAt    time.Time `json:"created_at,omitempty" db:"created_at"`
}

var conversation string = uuid.New().String()

type Response struct {
	Name    string `json:"name,omitempty"`
	Tone    string `json:"tone,omitempty"`
	Content string `json:"content,omitempty"`
}

func main() {
	app := &cli.App{
		Name:  "coppermind-twilio-bridge",
		Usage: "Twilio HTTP Bridge for Coppermind",
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Start the Twilio bridge server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Value: "",
						Usage: "file path to the numbers file",
					},
				},
				Action: func(cli *cli.Context) error {
					args := cli.Args()
					numbersFile := cli.String("file")
					if numbersFile == "" {
						log.Fatal("Numbers file must be provided")
					}

					port := args.Get(1)
					if port == "" {
						port = "6000"
					}

					err := loadNumbers(numbersFile)
					if err != nil {
						log.Fatal("Could not load numbers file", err)
					}

					serve(port)
					return nil
				},
			},
			{
				Name:  "add",
				Usage: "Add a person + phone number to the bridge",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Value: "",
						Usage: "file path to the numbers file",
					},
				},
				Action: func(cli *cli.Context) error {
					args := cli.Args()
					name := args.Get(0)
					phone := args.Get(1)

					if name == "" || phone == "" {
						log.Fatal("Name and phone number must be provided in that order")
					}

					numbersFile := cli.String("file")
					if numbersFile == "" {
						log.Fatal("Numbers file must be provided")
					}

					err := loadNumbers(numbersFile)
					if err != nil {
						log.Fatal("Could not load numbers file", err)
					}

					addPerson(name, phone)

					return nil
				},
			},
			{
				Name:  "remove",
				Usage: "Remove a person from the bridge",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "file",
						Value: "",
						Usage: "file path to the numbers file",
					},
				},
				Action: func(cli *cli.Context) error {
					args := cli.Args()
					name := args.Get(0)

					if name == "" {
						log.Fatal("Name must be provided in that order")
					}

					numbersFile := cli.String("file")
					if numbersFile == "" {
						log.Fatal("Numbers file must be provided")
					}

					err := loadNumbers(numbersFile)
					if err != nil {
						log.Fatal("Could not load numbers file", err)
					}

					removePerson(name)

					return nil
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func serve(port string) {
	accountSid := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	if accountSid == "" || authToken == "" {
		log.Fatal("TWILIO_ACCOUNT_SID and TWILIO_AUTH_TOKEN must be set")
	}

	client = twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})

	router := mux.NewRouter()

	// Define a handler function to process incoming SMS messages

	// Add a route to the router for handling incoming SMS messages
	router.HandleFunc("/sms", handleMessage).Methods("POST")

	// Start the server and listen for incoming requests
	log.Printf("Starting server on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func loadNumbers(filepath string) error {
	raw, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(raw, &numbers)
	if err != nil {
		return err
	}

	return nil
}

func saveNumbers(filepath string) error {
	raw, err := json.Marshal(numbers)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath, raw, 0644)
	return err
}

func addPerson(name, phone string) {
	numbers[name] = phone

	err := saveNumbers("numbers.json")
	if err != nil {
		log.Fatal("Could not save number to file", err)
	}
}

func removePerson(name string) {
	delete(numbers, name)

	err := saveNumbers("numbers.json")
	if err != nil {
		log.Fatal("Could not save number to file", err)
	}
}

func handleMessage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println("Error parsing message data:", err)
		return
	}

	// Get the message body and sender phone number from the request
	body := r.PostForm.Get("Body")
	from := r.PostForm.Get("From")

	var name string

	if _, ok := numbers[from]; !ok {
		log.Println("Unknown number", from)
		return
	} else {
		name = numbers[from]
	}

	// Log the message information
	log.Printf("Received message from %s | %s: %s\n", from, name, body)

	msg := &Message{
		ID:           uuid.New().String(),
		Conversation: conversation,
		User:         name,
		Agent:        "Rose",
		Content:      body,
		Tone:         "",
		CreatedAt:    time.Now(),
	}
	response, err := SendMessage(msg)
	if err != nil {
		log.Println("Error thrown sending message", err)
		return
	}

	// Send a reply back to the sender
	twilioNumber := os.Getenv("TWILIO_PHONE_NUMBER")
	resp, err := client.Api.CreateMessage(&api.CreateMessageParams{
		To:   &from,
		From: &twilioNumber,
		Body: &response.Content,
	})
	if err != nil {
		log.Println("Error creating sms response", err)
		return
	} else {
		if resp.Sid != nil {
			log.Println(*resp.Sid)
		} else {
			log.Println(resp.Sid)
		}
	}

	log.Printf("Sent reply to %s\n", from)
}

func SendMessage(msg *Message) (*Response, error) {
	url := "http://localhost:8080/chat/send"
	jsonPayload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	fmt.Println("payload prepped")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	fmt.Println("creating client")

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	fmt.Println("response received")
	fmt.Println(">>", resp.Status, resp.Body)

	var response Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		panic(err)
	}

	return &response, nil
}
