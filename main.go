package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type App struct {
	Router *mux.Router
	DB *sql.DB
}

type RecordsRow struct {
	Id int
	Text string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateRecordBody struct {
	Text string
}

type ResponseMessage struct {
	Message string `json:"message"`
}

func (app *App) Initialize(user, password, dbname string) {
	connectionStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", user, password, dbname)

	var err error
	app.DB, err = sql.Open("postgres", connectionStr)
	if err != nil {
		panic(err)
	}

	app.Router = mux.NewRouter()
	app.setRouters()
}

func (app *App) setRouters() {
	app.Get("/records", app.getRecords)
	app.Post("/records", app.createRecord)
}

func (app *App) Get(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("GET")
}

func (app *App) Post(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("POST")
}

func (app *App) getRecords(writer http.ResponseWriter, request *http.Request) {
	rows, err := app.DB.Query("SELECT * FROM records")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var row RecordsRow
		if err := rows.Scan(&row.Id, &row.Text, &row.CreatedAt, &row.UpdatedAt); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(writer, "%d | %s | %v | %v\n", row.Id, row.Text, row.CreatedAt, row.UpdatedAt)
	}

	if err = rows.Err(); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) createRecord(writer http.ResponseWriter, request *http.Request) {
	var requestBody CreateRecordBody
	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
	}

	if requestBody.Text == "" {
		respondWithJSON(writer, http.StatusBadRequest, ResponseMessage{ Message: "'text' field is required!" })
		return
	}

	_, err := app.DB.Exec("INSERT INTO records (text) VALUES ($1)", requestBody.Text)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	respondWithJSON(writer, http.StatusOK, ResponseMessage{ Message: "Record successfully added." })
}

func respondWithJSON(writer http.ResponseWriter, statusCode int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	writer.Write(response)
}

func (app *App) Run(port string) {
	fmt.Println("Starting server at :8080")
	http.ListenAndServe(port, app.Router)
}

func main() {
	app := &App{}
	app.Initialize("erik", "erik", "sib")
	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt)
	go func() {
		<-channel
		fmt.Println("WHY THE FUCK WOULD YOU DO THAT?!!")
		os.Exit(0)
	}()
	app.Run(":8080")
}