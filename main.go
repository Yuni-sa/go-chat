package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
)

// the chat message structre
type ChatMessage struct {
	Room     string `json:"room"`
	Id       string `json:"id"`
	Username string `json:"username"`
	Text     string `json:"text"`
	Time     string `json:"time"`
}

// declares rdb as type *redis.client
var (
	rdb *redis.Client
)

var clients = make(map[*websocket.Conn]bool)
var broadcaster = make(chan ChatMessage)
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print(err)
	}

	// logs connections
	log.Println("Client Connected, there are now ", len(clients)+1, "clients connected")

	// ensure connection close when function returns
	defer ws.Close()
	clients[ws] = true

	// checks number of sent message, if = to 0 then its empty
	if rdb.Exists("chat_messages").Val() != 0 {
		sendPreviousMessages(ws)
	}

	for {

		var msg ChatMessage
		// read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			delete(clients, ws)
			break
		}
		// send new message to the channel
		currentTime := time.Now()
		msg.Time = currentTime.Format("2006-01-02 3:4:5 pm")
		pmsgd := &msg
		pmsg, _ := json.Marshal(pmsgd)
		log.Println(string(pmsg))
		broadcaster <- msg
	}
}

func sendPreviousMessages(ws *websocket.Conn) {
	chatMessages, err := rdb.LRange("chat_messages", 0, -1).Result()
	if err != nil {
		panic(err)
	}

	// send previous messages
	for _, chatMessage := range chatMessages {
		var msg ChatMessage
		json.Unmarshal([]byte(chatMessage), &msg)
		messageClient(ws, msg)
	}
}

// If a message is sent while a client is closing, ignore the error
func unsafeError(err error) bool {
	return !websocket.IsCloseError(err, websocket.CloseGoingAway) && err != io.EOF
}

// takes the message from the channel, stores and sends it
func handleMessages() {
	for {
		// grabs messages from channel
		msg := <-broadcaster

		// sends the message to redis
		sendToRedis(msg)

		// sends the message to the clients
		messageClients(msg)
	}
}

func sendToRedis(msg ChatMessage) {
	// parses the JSON-encoded data into json var
	json, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	// pushes the message to redis
	if err := rdb.RPush("chat_messages", json).Err(); err != nil {
		panic(err)
	}
}

func messageClients(msg ChatMessage) {
	// send to every client currently connected
	for client := range clients {
		messageClient(client, msg)
	}
}

func messageClient(client *websocket.Conn, msg ChatMessage) {
	// sends the message and returns error
	err := client.WriteJSON(msg)

	// if there is and the connection is not close; log the error and remove client
	if err != nil && unsafeError(err) {
		log.Printf("error: %v", err)
		client.Close()
		delete(clients, client)
	}
}

func main() {
	port := "8000"
	// port := os.Getenv("PORT")

	// redisURL := "redis://:localhost:6379"
	// redisURL := "redis://:172.17.0.2:6379"
	// _, err := redis.ParseURL(redisURL)
	// if err != nil {
	// 	panic(err)
	// }

	// defines redis connection
	rdb = redis.NewClient(&redis.Options{
		// Addr: "localhost:6379",
		// Addr:     "redis:6379",
		Addr:     "redis-service.socialhub.svc.cluster.local:6379",
		Password: "",
		DB:       0,
	})

	// simple ping / connection check
	pong, err := rdb.Ping().Result()
	log.Println(pong, err)

	// creates an echo server
	// http.Handle("/", http.FileServer(http.Dir("./public")))

	// handles connection through /websocket
	http.HandleFunc("/websocket", handleConnections)
	go handleMessages()

	log.Print("Server starting at localhost:" + port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
