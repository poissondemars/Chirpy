package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/poissondemars/Chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries *database.Queries
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")

	fmt.Fprintf(w, `
	<html>
		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %d times!</p>
		</body>
	</html>
	`, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handleResetMetrics(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
}


func (cfg *apiConfig) handleChirpCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
		UserId string `json:"user_id"`
	}

	type returnVals struct {
		Id string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserId string `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	userId, err := uuid.Parse(params.UserId)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	chirpParams := database.CreateChirpParams{
		Body: sql.NullString{String: params.Body, Valid: true},
		UserID: uuid.NullUUID{UUID: userId, Valid: true},
	}
	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), chirpParams,)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.Time.GoString(),
		UpdatedAt: chirp.UpdatedAt.Time.GoString(),
		Body: chirp.Body.String,
		UserId: chirp.UserID.UUID.String(),
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)
}

func (cfg *apiConfig) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	type returnVals struct {
		Id string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), sql.NullString{String: params.Email, Valid: true})
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: user.ID.String(),
		CreatedAt: user.CreatedAt.Time.GoString(),
		UpdatedAt: user.UpdatedAt.Time.GoString(),
		Email: user.Email.String,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)
}

func main() {
	godotenv.Load()

	// Setting up DB
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	// Setting up server
	mux := http.NewServeMux()
	apiConfig := &apiConfig{
		dbQueries: database.New(db),
	}

	// App
	mux.Handle("/app/", middlewareLog(apiConfig.middlewareMetricsInc(http.FileServer(http.Dir(".")))))

	// Admin
	mux.Handle("GET /admin/metrics", middlewareLog(http.HandlerFunc(apiConfig.handleMetrics)))
	mux.Handle("POST /admin/reset", middlewareLog(http.HandlerFunc(apiConfig.handleResetMetrics)))

	// API
	mux.Handle("GET /api/healthz", middlewareLog(http.HandlerFunc(handleHealthz)))
	mux.Handle("POST /api/validate_chirp", middlewareLog(http.HandlerFunc(handleValidateChirp)))

	mux.Handle("POST /api/users", middlewareLog(http.HandlerFunc(apiConfig.handleUserCreate)))
	mux.Handle("POST /api/chirps", middlewareLog(http.HandlerFunc(apiConfig.handleChirpCreate)))

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	server.ListenAndServe()
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")

	w.WriteHeader(200)
	w.Write([]byte(`OK`))
}

func handleValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}

	type errorResp struct {
		Error string `json:"error"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		errorResp := errorResp{
			Error: "Something went wrong",
		}
		data, err := json.Marshal(errorResp)

		if err != nil {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		w.WriteHeader(500)

		return
	}

	chirpLength := utf8.RuneCountInString(params.Body)
	log.Printf("%s (%d)", params.Body, chirpLength)

	if chirpLength > 140 {
		errorResp := errorResp{
			Error: "Chirp is too long",
		}
		data, err := json.Marshal(errorResp)

		if err != nil {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(data)

		return
	}

	profaneDict := map[string]bool{
		"kerfuffle": true,
		"sharbert": true,
		"fornax": true,
	}
	chirpWords := strings.Split(params.Body, " ")
	for i, word := range chirpWords {
		if profaneDict[strings.ToLower(word)] {
			chirpWords[i] = "****"
		}
	}
	cleanedChirp := strings.Join(chirpWords, " ")

	returnVal := returnVals{
		CleanedBody: cleanedChirp,
	}

	data, err := json.Marshal(returnVal)
	defer w.WriteHeader(200)
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
