package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

type AuthenticateBody struct {
	Otp string
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
	app.Post("/authenticate", app.withCORS(app.authenticate))
	app.Get("/is-authenticated", app.withCORS(app.isAuthenticated))
	app.Get("/records", app.withCORS(app.protected(app.getRecords)))
	app.Get("/records/{id}", app.withCORS(app.protected(app.getRecord)))
	app.Post("/records", app.withCORS(app.protected(app.createRecord)))
	app.Put("/records/{id}", app.withCORS(app.protected(app.updateRecord)))
	app.Delete("/records/{id}", app.withCORS(app.protected(app.deleteRecord)))
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

func (app *App) protected(handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		authHeader := request.Header.Get("Authorization")
		if authHeader == "" {
			respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
			return
		}
		
		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 {
			respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
			return
		}

		token, err := jwt.Parse(bearerToken[1], func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(os.Getenv("SIB_API_JWT_SECRET")), nil
		})

		if err != nil {
			respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
			return
		}

		if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			handler.ServeHTTP(writer, request)
		} else {
			respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
			return
		}
	}
}

func (app *App) withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		handler.ServeHTTP(writer, request)
	}
}

func (app *App) authenticate(writer http.ResponseWriter, request *http.Request) {
	var requestBody AuthenticateBody;
	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": err.Error() })
		return
	}
	defer request.Body.Close()

	if requestBody.Otp == "" {
		respondWithJSON(writer, http.StatusBadRequest, responseJson{ "message": "'otp' field is required" })
		return
	}
	var used bool;
	var expiresAt time.Time
	// TODO: add bcrypt here
	err := app.DB.QueryRow("SELECT used, expiresat FROM otps WHERE otp = $1", requestBody.Otp).Scan(&used, &expiresAt)
	// if err != nil || used || time.Now().After(expiresAt) {
	// 	respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
	// 	return
	// }
	if err != nil {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Err not nil." })
		return
	}
	if used {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Used" })
		return
	}
	if time.Now().After(expiresAt) {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Expired" })
		return
	}
	
	_, err = app.DB.Exec("UPDATE otps SET used = true WHERE otp = $1", requestBody.Otp)
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	token, err := generateJWT()
	if err != nil {
		respondWithJSON(writer, http.StatusInternalServerError, responseJson{ "message": err.Error() })
		return
	}

	respondWithJSON(writer, http.StatusOK, responseJson{ "token": token })
}

func (app *App) isAuthenticated(writer http.ResponseWriter, request *http.Request) {
	authHeader := request.Header.Get("Authorization")
	if authHeader == "" {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
		return
	}
	
	bearerToken := strings.Split(authHeader, " ")
	if len(bearerToken) != 2 {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
		return
	}

	token, err := jwt.Parse(bearerToken[1], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("SIB_API_JWT_SECRET")), nil
	})

	if err != nil {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
		return
	}

	if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		respondWithJSON(writer, http.StatusOK, responseJson{ "authenticated": "true" })
	} else {
		respondWithJSON(writer, http.StatusUnauthorized, responseJson{ "message": "Access denied." })
		return
	}
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
	defer request.Body.Close()

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
	defer request.Body.Close()

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

func generateJWT() (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 365 * 100)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(os.Getenv("SIB_API_JWT_SECRET")))
	return tokenString, err
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
	app.Initialize("erik", os.Getenv("SIB_API_DB_PASS"), "sib")
	channel := make(chan os.Signal, 1)
	signal.Notify(channel, os.Interrupt)
	go func() {
		<-channel
		fmt.Println("\nBye!")
		os.Exit(0)
	}()
	app.Run(":8080")
}