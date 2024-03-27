package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type App struct {
	Router *mux.Router
	DB *sql.DB
}

type RecordsRow struct {
	Id int `json:"id"`
	Text string `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RecordBody struct {
	Text string
}

type responseJson map[string]string

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
	app.Get("/records/{id}", app.getRecord)
	app.Post("/records", app.createRecord)
	app.Put("/records/{id}", app.updateRecord)
	app.Delete("/records/{id}", app.deleteRecord)
}

func (app *App) Get(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("GET")
}

func (app *App) Post(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("POST")
}

func (app *App) Put(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("PUT")
}

func (app *App) Delete(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("DELETE")
}

func (app *App) getRecords(writer http.ResponseWriter, request *http.Request) {
	var records []RecordsRow

	rows, err := app.DB.Query("SELECT * FROM records")
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}
	defer rows.Close()

	for rows.Next() {
		var row RecordsRow
		if err := rows.Scan(&row.Id, &row.Text, &row.CreatedAt, &row.UpdatedAt); err != nil {
			respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
			return
		}
		records = append(records, row)
	}

	if err = rows.Err(); err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	respondWithJSON(writer, http.StatusOK, records)
}

func (app *App) createRecord(writer http.ResponseWriter, request *http.Request) {
	var requestBody RecordBody
	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": err.Error() })
		return
	}

	if requestBody.Text == "" {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'text' field is required!" })
		return
	}

	_, err := app.DB.Exec("INSERT INTO records (text) VALUES ($1)", requestBody.Text)
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "error": err.Error() })
		return
	}

	respondWithJSON(writer, http.StatusOK, map[string]string{ "message": "Record successfully added." })
}

func (app *App) updateRecord(writer http.ResponseWriter, request *http.Request) {
	var requestBody RecordBody

	id, err := strconv.Atoi(mux.Vars(request)["id"])
	if err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'id' must be of type int" })
		return
	}

	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": err.Error() })
		return
	}

	if requestBody.Text == "" {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'text' field is required" })
		return
	}

	_, err = app.DB.Exec("UPDATE records SET text = $1, updatedat = NOW() WHERE id = $2", requestBody.Text, id)
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	respondWithJSON(writer, http.StatusOK, responseJson{ "message": "Record successfully updated." })
}

func (app *App) getRecord(writer http.ResponseWriter, request * http.Request) {
	var record RecordsRow

	id, err := strconv.Atoi(mux.Vars(request)["id"])
	if err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'id' must be of type int" })
		return
	}

	rows, err := app.DB.Query("SELECT * FROM records WHERE id = $1", id)
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&record.Id, &record.Text, &record.CreatedAt, &record.UpdatedAt); err != nil {
			respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
			return
		}
	} else {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": fmt.Sprintf("record with id %d does not exist!", id) })
		return
	}

	respondWithJSON(writer, http.StatusOK, record)
}

func (app *App) deleteRecord(writer http.ResponseWriter, request *http.Request) {
	id, err := strconv.Atoi(mux.Vars(request)["id"])
	if err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'id' must be of type int" })
		return
	}

	result, err := app.DB.Exec("DELETE FROM records WHERE id = $1", id)
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	if rowsAffected == 0 {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": fmt.Sprintf("record with id %d does not exist!", id) })
		return
	}

	respondWithJSON(writer, http.StatusOK, responseJson{ "message": "record deleted successfully." })
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