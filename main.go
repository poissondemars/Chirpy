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
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/poissondemars/Chirpy/internal/database"
	"github.com/poissondemars/Chirpy/internal/auth"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries *database.Queries
	jwtSecret string
}

func (cfg *apiConfig) middlewareAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwtToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			w.WriteHeader(401)
			return
		}

		_, err = auth.ValidateJWT(jwtToken, cfg.jwtSecret)
		if err != nil {
			w.WriteHeader(401)
			return
		}

		next.ServeHTTP(w, r)
	})
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

func (cfg *apiConfig) handleGetChirp(w http.ResponseWriter, r *http.Request) {
	type returnVals struct {
		Id string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserId string `json:"user_id"`
	}
	
	chirpId, err := uuid.Parse(r.PathValue("chirpId"))
	if err != nil {
		w.WriteHeader(400)
		return
	}

	chirp, err := cfg.dbQueries.GetChirp(r.Context(), chirpId)
	if err != nil {
		w.WriteHeader(404)
		return
	}

	returnVal := returnVals{
		Id: chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: chirp.UpdatedAt.Time.Format(time.RFC3339),
		Body: chirp.Body.String,
		UserId: chirp.UserID.UUID.String(),
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func (cfg *apiConfig) handleGetChirps(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Id string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserId string `json:"user_id"`
	}

	chirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		w.WriteHeader(500)
		return
	}

	response := []chirp{}
	for _, c := range chirps {
		response = append(response, chirp{
			Id: c.ID.String(),
			CreatedAt: c.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt: c.UpdatedAt.Time.Format(time.RFC3339),
			Body: c.Body.String,
			UserId: c.UserID.UUID.String(),
		})
	}

	data, err := json.Marshal(response)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func (cfg *apiConfig) handleChirpCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
		UserId string `json:"user_id"`
	}

	type errorResp struct {
		Error string `json:"error"`
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

	chirpParams := database.CreateChirpParams{
		Body: sql.NullString{String: cleanedChirp, Valid: true},
		UserID: uuid.NullUUID{UUID: userId, Valid: true},
	}
	chirp, err := cfg.dbQueries.CreateChirp(r.Context(), chirpParams,)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: chirp.UpdatedAt.Time.Format(time.RFC3339),
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
		Password string `json:"password"`
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

	hashedPassword, err := auth.HashPassword(params.Password)	
	if err != nil {
		w.WriteHeader(400)
		return
	}
	createUserParams := database.CreateUserParams{
		Email: sql.NullString{String: params.Email, Valid: true},
		HashedPassword: hashedPassword,
	}
	user, err := cfg.dbQueries.CreateUser(
		r.Context(), 
		createUserParams,
	)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: user.ID.String(),
		CreatedAt: user.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.Time.Format(time.RFC3339),
		Email: user.Email.String,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)
}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
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

	user, err := cfg.dbQueries.GetUserByEmail(
		r.Context(), 
		sql.NullString{String: params.Email, Valid: true},
	)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	passwordMatch, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	if !passwordMatch {
		w.WriteHeader(401)
		w.Write([]byte(`Incorrect email or password`))
		return
	}

	returnVal := returnVals{
		Id: user.ID.String(),
		CreatedAt: user.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.Time.Format(time.RFC3339),
		Email: user.Email.String,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
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
	jwtSecret := os.Getenv("JWT_SECRET")
	apiConfig := &apiConfig{
		dbQueries: database.New(db),
		jwtSecret: jwtSecret,
	}

	// App
	mux.Handle("/app/", middlewareLog(apiConfig.middlewareMetricsInc(http.FileServer(http.Dir(".")))))

	// Admin
	mux.Handle("GET /admin/metrics", middlewareLog(http.HandlerFunc(apiConfig.handleMetrics)))
	mux.Handle("POST /admin/reset", middlewareLog(http.HandlerFunc(apiConfig.handleResetMetrics)))

	// API
	mux.Handle("GET /api/healthz", middlewareLog(http.HandlerFunc(handleHealthz)))

	// Users
	mux.Handle("POST /api/users", middlewareLog(http.HandlerFunc(apiConfig.handleUserCreate)))
	mux.Handle("POST /api/login", middlewareLog(http.HandlerFunc(apiConfig.handleLogin)))

	// Chirps
	mux.Handle("POST /api/chirps", middlewareLog(apiConfig.middlewareAuth(http.HandlerFunc(apiConfig.handleChirpCreate))))
	mux.Handle("GET /api/chirps", middlewareLog(http.HandlerFunc(apiConfig.handleGetChirps)))
	mux.Handle("GET /api/chirps/{chirpId}", middlewareLog(http.HandlerFunc(apiConfig.handleGetChirp)))

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

func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
