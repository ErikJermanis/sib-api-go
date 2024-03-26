package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type App struct {
	Router *mux.Router
	DB *sql.DB
}

type TestRow struct {
	Id int
	Text string
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
	app.Get("/getTest", app.getTest)
}

func (app *App) Get(path string, handler func(writer http.ResponseWriter, request *http.Request)) {
	app.Router.HandleFunc(path, handler).Methods("GET")
}

func (app *App) getTest(writer http.ResponseWriter, request *http.Request) {
	rows, err := app.DB.Query("SELECT * FROM test")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var row TestRow
		if err := rows.Scan(&row.Id, &row.Text); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(writer, "Id: %d, Text: %s\n", row.Id, row.Text)
	}

	if err = rows.Err(); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
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