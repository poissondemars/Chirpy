package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"errors"
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
	polkaKey string
}

func (cfg *apiConfig) checkUserAuth(r *http.Request) (uuid.UUID, error) {
	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		return uuid.UUID{}, errors.New("failed to extract bearer token")
	}

	userUUID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil {
		return uuid.UUID{}, errors.New("failed to validate token")
	}

	return userUUID, nil
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

func (cfg *apiConfig) handleReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)

	err := cfg.dbQueries.RefreshTokensDeleteAll(r.Context())
	if err != nil {
		log.Printf("Failed to reset RefreshTokens: %v", err)
		w.WriteHeader(400)
		return
	}

	err = cfg.dbQueries.ChirpsDeleteAll(r.Context())
	if err != nil {
		log.Printf("Failed to reset Chirps: %v", err)
		w.WriteHeader(400)
		return
	}

	err = cfg.dbQueries.UsersDeleteAll(r.Context())
	if err != nil {
		log.Printf("Failed to reset Users: %v", err)
		w.WriteHeader(400)
		return
	}

	w.WriteHeader(200)
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

func (cfg *apiConfig) handleDeleteChirp(w http.ResponseWriter, r *http.Request) {
	userId, err := cfg.checkUserAuth(r)
	if err != nil {
		w.WriteHeader(401)
		return
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

	if userId != chirp.UserID.UUID {
		w.WriteHeader(403)
		return
	}

	// Delete
	err = cfg.dbQueries.DeleteChirp(r.Context(), chirp.ID)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	w.WriteHeader(204)
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

	userId, err := cfg.checkUserAuth(r)
	if err != nil {
		w.WriteHeader(401)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
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
		IsChirpyRed bool `json:"is_chirpy_red"`
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
		IsChirpyRed: user.IsChirpyRed,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)
}

func (cfg *apiConfig) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}

	type returnVals struct {
		Id string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email string `json:"email"`
		IsChirpyRed bool `json:"is_chirpy_red"`
	}

	userId, err := cfg.checkUserAuth(r)
	if err != nil {
		w.WriteHeader(401)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)	
	if err != nil {
		w.WriteHeader(400)
		return
	}

	updateParams := database.UpdateUserParams{
		ID: 			userId,
		Email:			sql.NullString{String: params.Email, Valid: true},
		HashedPassword: hashedPassword,
	}
	updatedUser, err := cfg.dbQueries.UpdateUser(r.Context(), updateParams)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: updatedUser.ID.String(),
		CreatedAt: updatedUser.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: updatedUser.UpdatedAt.Time.Format(time.RFC3339),
		Email: updatedUser.Email.String,
		IsChirpyRed: updatedUser.IsChirpyRed,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
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
		Token string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		IsChirpyRed bool `json:"is_chirpy_red"`
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

	expiresIn := time.Duration(1 * 60 * 60 * time.Second)
	jwtToken, err :=  auth.MakeJWT(user.ID, cfg.jwtSecret, expiresIn)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	refreshTokenParams := database.CreateRefreshTokenParams{
		Token: auth.MakeRefreshToken(),
		UserID: uuid.NullUUID{UUID: user.ID, Valid: true},
		ExpiresAt: sql.NullTime{Time: time.Now().Add(60 * 24 * time.Hour), Valid: true},
	}
	refreshToken, err := cfg.dbQueries.CreateRefreshToken(r.Context(), refreshTokenParams)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVal := returnVals{
		Id: user.ID.String(),
		CreatedAt: user.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.Time.Format(time.RFC3339),
		Email: user.Email.String,
		Token: jwtToken,
		RefreshToken: refreshToken.Token,
		IsChirpyRed: user.IsChirpyRed,
	}
	data, _ := json.Marshal(returnVal)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func (cfg *apiConfig) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	refreshTokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	refreshToken, err := cfg.dbQueries.GetRefreshToken(r.Context(), refreshTokenString)
	// Entity doesn't exist
	if err != nil {
		w.WriteHeader(401)
		return
	}
	// Token revoked
	if refreshToken.RevokedAt.Valid == true {
		w.WriteHeader(401)
		return
	}
	// Token revoked
	if refreshToken.ExpiresAt.Valid == false {
		w.WriteHeader(401)
		return
	}
	// Token expired
	if time.Now().After(refreshToken.ExpiresAt.Time) {
		w.WriteHeader(401)
		return
	}

	type returnValues struct {
		Token string `json:"token"`
	}

	// create new jwt
	expiresIn := time.Duration(1 * 60 * 60 * time.Second)
	newJwtToken, err := auth.MakeJWT(refreshToken.UserID.UUID, cfg.jwtSecret, expiresIn)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	returnVals := returnValues{
		Token: newJwtToken,
	}
	data, err := json.Marshal(returnVals)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func (cfg *apiConfig) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	refreshTokenString, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	refreshToken, err := cfg.dbQueries.GetRefreshToken(r.Context(), refreshTokenString)
	// Entity doesn't exist
	if err != nil {
		w.WriteHeader(401)
		return
	}
	// Token revoked
	if refreshToken.RevokedAt.Valid == true {
		w.WriteHeader(401)
		return
	}
	// Token revoked
	if refreshToken.ExpiresAt.Valid == false {
		w.WriteHeader(401)
		return
	}
	// Token expired
	if time.Now().After(refreshToken.ExpiresAt.Time) {
		w.WriteHeader(401)
		return
	}

	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), refreshTokenString)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) handlePolkaWebhook(w http.ResponseWriter, r *http.Request) {
	type userData struct {
		UserId uuid.UUID `json:"user_id"`
	}

	type parameters struct {
		Event string `json:"event"`
		UserData userData `json:"data"`
	}

	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		w.WriteHeader(401)
		return
	}
	if apiKey != cfg.polkaKey {
		w.WriteHeader(401)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}

	dbParams := database.UpdateUserToChirpyRedParams{
		ID: params.UserData.UserId,
		IsChirpyRed: true,
	}
	_, err = cfg.dbQueries.UpdateUserToChirpyRed(r.Context(), dbParams)
	if err != nil {
		w.WriteHeader(404)
		return
	}

	w.WriteHeader(204)
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
	polkaKey := os.Getenv("POLKA_KEY")
	apiConfig := &apiConfig{
		dbQueries: database.New(db),
		jwtSecret: jwtSecret,
		polkaKey: polkaKey,
	}

	// App
	mux.Handle("/app/", middlewareLog(apiConfig.middlewareMetricsInc(http.FileServer(http.Dir(".")))))

	// Admin
	mux.Handle("GET /admin/metrics", middlewareLog(http.HandlerFunc(apiConfig.handleMetrics)))
	mux.Handle("POST /admin/reset", middlewareLog(http.HandlerFunc(apiConfig.handleReset)))

	// API
	mux.Handle("GET /api/healthz", middlewareLog(http.HandlerFunc(handleHealthz)))

	// Users
	mux.Handle("POST /api/users", middlewareLog(http.HandlerFunc(apiConfig.handleUserCreate)))
	mux.Handle("PUT /api/users", middlewareLog(http.HandlerFunc(apiConfig.handleUserUpdate)))
	mux.Handle("POST /api/login", middlewareLog(http.HandlerFunc(apiConfig.handleLogin)))
	mux.Handle("POST /api/refresh", middlewareLog(http.HandlerFunc(apiConfig.handleRefreshToken)))
	mux.Handle("POST /api/revoke", middlewareLog(http.HandlerFunc(apiConfig.handleRevokeToken)))

	// Chirps
	mux.Handle("POST /api/chirps", middlewareLog(http.HandlerFunc(apiConfig.handleChirpCreate)))
	mux.Handle("GET /api/chirps", middlewareLog(http.HandlerFunc(apiConfig.handleGetChirps)))
	mux.Handle("GET /api/chirps/{chirpId}", middlewareLog(http.HandlerFunc(apiConfig.handleGetChirp)))
	mux.Handle("DELETE /api/chirps/{chirpId}", middlewareLog(http.HandlerFunc(apiConfig.handleDeleteChirp)))

	mux.Handle("POST /api/polka/webhooks", middlewareLog(http.HandlerFunc(apiConfig.handlePolkaWebhook)))

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
