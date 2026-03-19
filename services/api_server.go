package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"taskpilot/models"
	"time"

	"github.com/google/uuid"
)

// APIServer handles REST API endpoints for job management
type APIServer struct {
	jobService *JobService
	mcpServer  *MCPServer
	server     *http.Server
	port       int
}

// NewAPIServer creates a new API server instance
func NewAPIServer(jobService *JobService, port int) *APIServer {
	return &APIServer{
		jobService: jobService,
		mcpServer:  NewMCPServer(jobService),
		port:       port,
	}
}

// Start initializes and starts the HTTP server in a goroutine
func (s *APIServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Job endpoints
	mux.HandleFunc("/api/jobs", s.handleJobs)
	mux.HandleFunc("/api/jobs/", s.handleJobByID)

	// History endpoints
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/history/", s.handleHistoryByID)

	// System endpoints
	mux.HandleFunc("/api/system/time", s.handleSystemTime)
	mux.HandleFunc("/api/system/timezones", s.handleSystemTimezones)

	// MCP endpoints
	mux.HandleFunc("/api/mcp", s.handleMCP)

	// Swagger documentation endpoint
	mux.HandleFunc("/api/swagger", s.handleSwagger)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.enableCORS(mux),
	}

	go func() {
		log.Printf("Starting API server on port %d (MCP endpoint: http://localhost:%d/api/mcp)", s.port, s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the HTTP server
func (s *APIServer) Shutdown(ctx context.Context) error {
	if s.server != nil {
		log.Println("Shutting down API server")
		return s.server.Shutdown(ctx)
	}
	return nil
}

// enableCORS wraps the handler with CORS headers
func (s *APIServer) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleJobs routes GET (list) and POST (create) for /api/jobs
func (s *APIServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetJobs(w, r)
	case http.MethodPost:
		s.handleCreateJob(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJobByID routes GET, PUT, DELETE for /api/jobs/:id
func (s *APIServer) handleJobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if path == "" {
		s.sendError(w, "Job ID is required", http.StatusBadRequest)
		return
	}

	// Check if this is a request for job history
	if strings.HasSuffix(path, "/history") {
		jobID := strings.TrimSuffix(path, "/history")
		if r.Method == http.MethodGet {
			s.handleJobHistory(w, r, jobID)
		} else {
			s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// Check if this is a request to trigger a job
	if strings.HasSuffix(path, "/trigger") {
		jobID := strings.TrimSuffix(path, "/trigger")
		if r.Method == http.MethodPost {
			s.handleTriggerJob(w, r, jobID)
		} else {
			s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetJob(w, r, path)
	case http.MethodPut:
		s.handleUpdateJob(w, r, path)
	case http.MethodDelete:
		s.handleDeleteJob(w, r, path)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHistory routes GET (list) for /api/history
func (s *APIServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetHistory(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHistoryByID routes GET, DELETE for /api/history/:id
func (s *APIServer) handleHistoryByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/history/")
	if id == "" {
		s.sendError(w, "History ID is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetHistoryByID(w, r, id)
	case http.MethodDelete:
		s.handleDeleteHistory(w, r, id)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetJobs returns all jobs
// @Summary List all jobs
// @Description Get a list of all scheduled jobs
// @Tags jobs
// @Produce json
// @Success 200 {array} models.Job
// @Router /api/jobs [get]
func (s *APIServer) handleGetJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get jobs: %v", err), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, jobs, http.StatusOK)
}

// handleGetJob returns a specific job by ID
// @Summary Get job by ID
// @Description Get details of a specific job
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.Job
// @Failure 404 {object} map[string]string
// @Router /api/jobs/{id} [get]
func (s *APIServer) handleGetJob(w http.ResponseWriter, r *http.Request, id string) {
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get jobs: %v", err), http.StatusInternalServerError)
		return
	}

	for _, job := range jobs {
		if job.ID == id {
			s.sendJSON(w, job, http.StatusOK)
			return
		}
	}

	s.sendError(w, "Job not found", http.StatusNotFound)
}

// handleCreateJob creates a new job
// @Summary Create a new job
// @Description Create a new scheduled job
// @Tags jobs
// @Accept json
// @Produce json
// @Param job body models.Job true "Job object"
// @Success 201 {object} models.Job
// @Failure 400 {object} map[string]string
// @Router /api/jobs [post]
func (s *APIServer) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var job models.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		s.sendError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Generate ID if not provided
	if job.ID == "" {
		job.ID = uuid.New().String()
	}

	// Validate required fields
	if job.Name == "" || job.Command == "" || job.Schedule == "" {
		s.sendError(w, "Name, command, and schedule are required", http.StatusBadRequest)
		return
	}

	// Schedule type, cron syntax, and datetime validation are performed by jobService.CreateJob
	// via ValidateJob (see services/validation.go) to keep validation in one place across all callers.
	createdJob, err := s.jobService.CreateJob(job)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to create job: %v", err), http.StatusBadRequest)
		return
	}

	s.sendJSON(w, createdJob, http.StatusCreated)
}

// handleUpdateJob updates an existing job
// @Summary Update a job
// @Description Update an existing job by ID
// @Tags jobs
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Param job body models.Job true "Job object"
// @Success 200 {object} models.Job
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/jobs/{id} [put]
func (s *APIServer) handleUpdateJob(w http.ResponseWriter, r *http.Request, id string) {
	var job models.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		s.sendError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Ensure ID matches
	job.ID = id

	// Validate required fields
	if job.Name == "" || job.Command == "" || job.Schedule == "" {
		s.sendError(w, "Name, command, and schedule are required", http.StatusBadRequest)
		return
	}

	// Schedule type, cron syntax, and datetime validation are performed by jobService.UpdateJob
	// via ValidateJob (see services/validation.go) to keep validation in one place across all callers.

	// Check if job exists
	jobs, err := s.jobService.GetJobs()
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get jobs: %v", err), http.StatusInternalServerError)
		return
	}

	found := false
	for _, j := range jobs {
		if j.ID == id {
			found = true
			break
		}
	}

	if !found {
		s.sendError(w, "Job not found", http.StatusNotFound)
		return
	}

	updatedJob, err := s.jobService.UpdateJob(job)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to update job: %v", err), http.StatusBadRequest)
		return
	}

	s.sendJSON(w, updatedJob, http.StatusOK)
}

// handleDeleteJob deletes a job and its history
// @Summary Delete a job
// @Description Delete a job by ID (cascade deletes history)
// @Tags jobs
// @Param id path string true "Job ID"
// @Success 204 "No Content"
// @Failure 404 {object} map[string]string
// @Router /api/jobs/{id} [delete]
func (s *APIServer) handleDeleteJob(w http.ResponseWriter, r *http.Request, id string) {
	err := s.jobService.DeleteJob(id)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to delete job: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleTriggerJob triggers a job to execute immediately
// @Summary Trigger job execution
// @Description Trigger a job to execute immediately without waiting for its schedule
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/jobs/{id}/trigger [post]
func (s *APIServer) handleTriggerJob(w http.ResponseWriter, r *http.Request, id string) {
	err := s.jobService.TriggerJob(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.sendError(w, err.Error(), http.StatusNotFound)
		} else {
			s.sendError(w, fmt.Sprintf("Failed to trigger job: %v", err), http.StatusInternalServerError)
		}
		return
	}

	s.sendJSON(w, map[string]string{"message": "Job triggered successfully"}, http.StatusOK)
}

// handleGetHistory returns all history records
// @Summary List all history records
// @Description Get a list of all job execution history records with optional filtering and ordering
// @Tags history
// @Produce json
// @Param job_id query string false \"Filter by job ID (UUID)\"
// @Param order query string false \"Order by timestamp: asc or desc (default: desc)\"
// @Success 200 {array} models.History
// @Failure 400 {object} map[string]string
// @Router /api/history [get]
func (s *APIServer) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	queryParams := r.URL.Query()
	var jobID *string
	orderAsc := false

	// Parse job_id parameter
	if jobIDParam := queryParams.Get("job_id"); jobIDParam != "" {
		// Validate UUID format
		if !ValidateUUID(jobIDParam) {
			s.sendError(w, "Invalid job_id format: must be a valid UUID", http.StatusBadRequest)
			return
		}
		jobID = &jobIDParam
	}

	// Parse order parameter (default to desc)
	if orderParam := queryParams.Get("order"); orderParam == "asc" {
		orderAsc = true
	}
	// If order is not "asc", default to desc (including invalid values)

	history, err := s.jobService.GetHistory(jobID, orderAsc)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get history: %v", err), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, history, http.StatusOK)
}

// handleJobHistory returns history records for a specific job
// @Summary Get history for a specific job
// @Description Get all history records for a specific job with optional ordering
// @Tags history
// @Produce json
// @Param id path string true "Job ID (UUID)"
// @Param order query string false "Order by timestamp: asc or desc (default: desc)"
// @Success 200 {array} models.History
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/jobs/{id}/history [get]
func (s *APIServer) handleJobHistory(w http.ResponseWriter, r *http.Request, jobID string) {
	// Validate job_id format
	if !ValidateUUID(jobID) {
		s.sendError(w, "Invalid job_id format: must be a valid UUID", http.StatusBadRequest)
		return
	}

	// Check if job exists
	_, err := s.jobService.GetJobByID(jobID)
	if err != nil {
		s.sendError(w, "Job not found", http.StatusNotFound)
		return
	}

	// Parse order parameter (default to desc)
	orderAsc := false
	if orderParam := r.URL.Query().Get("order"); orderParam == "asc" {
		orderAsc = true
	}

	// Get history for this specific job
	history, err := s.jobService.GetHistory(&jobID, orderAsc)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get job history: %v", err), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, history, http.StatusOK)
}

// handleGetHistoryByID returns a specific history record
// @Summary Get history record by ID
// @Description Get details of a specific history record
// @Tags history
// @Produce json
// @Param id path string true "History ID"
// @Success 200 {object} models.History
// @Failure 404 {object} map[string]string
// @Router /api/history/{id} [get]
func (s *APIServer) handleGetHistoryByID(w http.ResponseWriter, r *http.Request, id string) {
	history, err := s.jobService.GetHistory(nil, false)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to get history: %v", err), http.StatusInternalServerError)
		return
	}

	for _, h := range history {
		if h.ID == id {
			s.sendJSON(w, h, http.StatusOK)
			return
		}
	}

	s.sendError(w, "History record not found", http.StatusNotFound)
}

// handleDeleteHistory deletes a history record
// @Summary Delete history record
// @Description Delete a specific history record by ID
// @Tags history
// @Param id path string true "History ID"
// @Success 204 "No Content"
// @Failure 404 {object} map[string]string
// @Router /api/history/{id} [delete]
func (s *APIServer) handleDeleteHistory(w http.ResponseWriter, r *http.Request, id string) {
	err := s.jobService.DeleteHistory(id)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Failed to delete history: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SystemTimeResponse represents the system time and timezone information
type SystemTimeResponse struct {
	Timestamp        int64  `json:"timestamp"`
	ISO8601          string `json:"iso8601"`
	Timezone         string `json:"timezone"`
	TimezoneOffset   string `json:"timezone_offset"`
	UTCOffsetSeconds int    `json:"utc_offset_seconds"`
}

// handleSystemTime returns current system time and timezone information
// @Summary Get system time and timezone
// @Description Returns the current server time in multiple formats along with timezone information
// @Tags system
// @Produce json
// @Success 200 {object} SystemTimeResponse
// @Router /api/system/time [get]
func (s *APIServer) handleSystemTime(w http.ResponseWriter, r *http.Request) {
	// Get current time
	now := time.Now()

	// Get timezone information
	zoneName, offset := now.Zone()

	// Create response
	response := SystemTimeResponse{
		Timestamp:        now.Unix(),
		ISO8601:          now.Format(time.RFC3339),
		Timezone:         zoneName,
		TimezoneOffset:   now.Format("-07:00"),
		UTCOffsetSeconds: offset,
	}

	s.sendJSON(w, response, http.StatusOK)
}

// TimezonesResponse is the payload returned by GET /api/system/timezones.
type TimezonesResponse struct {
	Current   string   `json:"current"`   // IANA name of the server's local timezone
	Timezones []string `json:"timezones"` // Sorted list of all well-known IANA timezone names
}

// handleSystemTimezones returns the server's current timezone and the full list
// of well-known IANA timezone names that clients may use (e.g. for job display).
//
// @Summary     List available timezones
// @Description Returns the server's current IANA timezone and a sorted list of all
// @Description well-known IANA timezone names.
// @Tags        system
// @Produce     json
// @Success     200 {object} TimezonesResponse
// @Router      /api/system/timezones [get]
func (s *APIServer) handleSystemTimezones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	zoneName, _ := time.Now().Zone()
	s.sendJSON(w, TimezonesResponse{
		Current:   zoneName,
		Timezones: ianaTimezones,
	}, http.StatusOK)
}

// ianaTimezones is a curated, sorted list of well-known IANA timezone names.
// Source: IANA Time Zone Database (https://www.iana.org/time-zones)
var ianaTimezones = []string{
	"Africa/Abidjan", "Africa/Accra", "Africa/Addis_Ababa", "Africa/Algiers",
	"Africa/Asmara", "Africa/Bamako", "Africa/Bangui", "Africa/Banjul",
	"Africa/Bissau", "Africa/Blantyre", "Africa/Brazzaville", "Africa/Bujumbura",
	"Africa/Cairo", "Africa/Casablanca", "Africa/Ceuta", "Africa/Conakry",
	"Africa/Dakar", "Africa/Dar_es_Salaam", "Africa/Djibouti", "Africa/Douala",
	"Africa/El_Aaiun", "Africa/Freetown", "Africa/Gaborone", "Africa/Harare",
	"Africa/Johannesburg", "Africa/Juba", "Africa/Kampala", "Africa/Khartoum",
	"Africa/Kigali", "Africa/Kinshasa", "Africa/Lagos", "Africa/Libreville",
	"Africa/Lome", "Africa/Luanda", "Africa/Lubumbashi", "Africa/Lusaka",
	"Africa/Malabo", "Africa/Maputo", "Africa/Maseru", "Africa/Mbabane",
	"Africa/Mogadishu", "Africa/Monrovia", "Africa/Nairobi", "Africa/Ndjamena",
	"Africa/Niamey", "Africa/Nouakchott", "Africa/Ouagadougou", "Africa/Porto-Novo",
	"Africa/Sao_Tome", "Africa/Tripoli", "Africa/Tunis", "Africa/Windhoek",
	"America/Adak", "America/Anchorage", "America/Anguilla", "America/Antigua",
	"America/Araguaina", "America/Argentina/Buenos_Aires", "America/Argentina/Catamarca",
	"America/Argentina/Cordoba", "America/Argentina/Jujuy", "America/Argentina/La_Rioja",
	"America/Argentina/Mendoza", "America/Argentina/Rio_Gallegos", "America/Argentina/Salta",
	"America/Argentina/San_Juan", "America/Argentina/San_Luis", "America/Argentina/Tucuman",
	"America/Argentina/Ushuaia", "America/Aruba", "America/Asuncion", "America/Atikokan",
	"America/Bahia", "America/Bahia_Banderas", "America/Barbados", "America/Belem",
	"America/Belize", "America/Blanc-Sablon", "America/Boa_Vista", "America/Bogota",
	"America/Boise", "America/Cambridge_Bay", "America/Campo_Grande", "America/Cancun",
	"America/Caracas", "America/Cayenne", "America/Cayman", "America/Chicago",
	"America/Chihuahua", "America/Ciudad_Juarez", "America/Costa_Rica", "America/Creston",
	"America/Cuiaba", "America/Curacao", "America/Danmarkshavn", "America/Dawson",
	"America/Dawson_Creek", "America/Denver", "America/Detroit", "America/Dominica",
	"America/Edmonton", "America/Eirunepe", "America/El_Salvador", "America/Fortaleza",
	"America/Glace_Bay", "America/Godthab", "America/Goose_Bay", "America/Grand_Turk",
	"America/Grenada", "America/Guadeloupe", "America/Guatemala", "America/Guayaquil",
	"America/Guyana", "America/Halifax", "America/Havana", "America/Hermosillo",
	"America/Indiana/Indianapolis", "America/Indiana/Knox", "America/Indiana/Marengo",
	"America/Indiana/Petersburg", "America/Indiana/Tell_City", "America/Indiana/Vevay",
	"America/Indiana/Vincennes", "America/Indiana/Winamac", "America/Inuvik",
	"America/Iqaluit", "America/Jamaica", "America/Juneau", "America/Kentucky/Louisville",
	"America/Kentucky/Monticello", "America/Kralendijk", "America/La_Paz", "America/Lima",
	"America/Los_Angeles", "America/Lower_Princes", "America/Maceio", "America/Managua",
	"America/Manaus", "America/Marigot", "America/Martinique", "America/Matamoros",
	"America/Mazatlan", "America/Menominee", "America/Merida", "America/Metlakatla",
	"America/Mexico_City", "America/Miquelon", "America/Moncton", "America/Monterrey",
	"America/Montevideo", "America/Montserrat", "America/Nassau", "America/New_York",
	"America/Nipigon", "America/Nome", "America/Noronha", "America/North_Dakota/Beulah",
	"America/North_Dakota/Center", "America/North_Dakota/New_Salem", "America/Nuuk",
	"America/Ojinaga", "America/Panama", "America/Pangnirtung", "America/Paramaribo",
	"America/Phoenix", "America/Port-au-Prince", "America/Port_of_Spain", "America/Porto_Velho",
	"America/Puerto_Rico", "America/Punta_Arenas", "America/Rainy_River", "America/Rankin_Inlet",
	"America/Recife", "America/Regina", "America/Resolute", "America/Rio_Branco",
	"America/Santa_Isabel", "America/Santarem", "America/Santiago", "America/Santo_Domingo",
	"America/Sao_Paulo", "America/Scoresbysund", "America/Sitka", "America/St_Barthelemy",
	"America/St_Johns", "America/St_Kitts", "America/St_Lucia", "America/St_Thomas",
	"America/St_Vincent", "America/Swift_Current", "America/Tegucigalpa", "America/Thule",
	"America/Thunder_Bay", "America/Tijuana", "America/Toronto", "America/Tortola",
	"America/Vancouver", "America/Whitehorse", "America/Winnipeg", "America/Yakutat",
	"America/Yellowknife",
	"Antarctica/Casey", "Antarctica/Davis", "Antarctica/DumontDUrville",
	"Antarctica/Macquarie", "Antarctica/Mawson", "Antarctica/McMurdo",
	"Antarctica/Palmer", "Antarctica/Rothera", "Antarctica/South_Pole",
	"Antarctica/Syowa", "Antarctica/Troll", "Antarctica/Vostok",
	"Arctic/Longyearbyen",
	"Asia/Aden", "Asia/Almaty", "Asia/Amman", "Asia/Anadyr", "Asia/Aqtau",
	"Asia/Aqtobe", "Asia/Ashgabat", "Asia/Atyrau", "Asia/Baghdad", "Asia/Bahrain",
	"Asia/Baku", "Asia/Bangkok", "Asia/Barnaul", "Asia/Beirut", "Asia/Bishkek",
	"Asia/Brunei", "Asia/Chita", "Asia/Choibalsan", "Asia/Colombo", "Asia/Damascus",
	"Asia/Dhaka", "Asia/Dili", "Asia/Dubai", "Asia/Dushanbe", "Asia/Famagusta",
	"Asia/Gaza", "Asia/Hebron", "Asia/Ho_Chi_Minh", "Asia/Hong_Kong", "Asia/Hovd",
	"Asia/Irkutsk", "Asia/Jakarta", "Asia/Jayapura", "Asia/Jerusalem", "Asia/Kabul",
	"Asia/Kamchatka", "Asia/Karachi", "Asia/Kathmandu", "Asia/Khandyga", "Asia/Kolkata",
	"Asia/Krasnoyarsk", "Asia/Kuala_Lumpur", "Asia/Kuching", "Asia/Kuwait",
	"Asia/Macau", "Asia/Magadan", "Asia/Makassar", "Asia/Manila", "Asia/Muscat",
	"Asia/Nicosia", "Asia/Novokuznetsk", "Asia/Novosibirsk", "Asia/Omsk",
	"Asia/Oral", "Asia/Phnom_Penh", "Asia/Pontianak", "Asia/Pyongyang",
	"Asia/Qatar", "Asia/Qostanay", "Asia/Qyzylorda", "Asia/Riyadh", "Asia/Sakhalin",
	"Asia/Samarkand", "Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Srednekolymsk",
	"Asia/Taipei", "Asia/Tashkent", "Asia/Tbilisi", "Asia/Tehran", "Asia/Thimphu",
	"Asia/Tokyo", "Asia/Tomsk", "Asia/Ulaanbaatar", "Asia/Urumqi", "Asia/Ust-Nera",
	"Asia/Vientiane", "Asia/Vladivostok", "Asia/Yakutsk", "Asia/Yangon",
	"Asia/Yekaterinburg", "Asia/Yerevan",
	"Atlantic/Azores", "Atlantic/Bermuda", "Atlantic/Canary", "Atlantic/Cape_Verde",
	"Atlantic/Faroe", "Atlantic/Madeira", "Atlantic/Reykjavik", "Atlantic/South_Georgia",
	"Atlantic/St_Helena", "Atlantic/Stanley",
	"Australia/Adelaide", "Australia/Brisbane", "Australia/Broken_Hill",
	"Australia/Darwin", "Australia/Eucla", "Australia/Hobart", "Australia/Lindeman",
	"Australia/Lord_Howe", "Australia/Melbourne", "Australia/Perth", "Australia/Sydney",
	"Europe/Amsterdam", "Europe/Andorra", "Europe/Astrakhan", "Europe/Athens",
	"Europe/Belgrade", "Europe/Berlin", "Europe/Bratislava", "Europe/Brussels",
	"Europe/Bucharest", "Europe/Budapest", "Europe/Busingen", "Europe/Chisinau",
	"Europe/Copenhagen", "Europe/Dublin", "Europe/Gibraltar", "Europe/Guernsey",
	"Europe/Helsinki", "Europe/Isle_of_Man", "Europe/Istanbul", "Europe/Jersey",
	"Europe/Kaliningrad", "Europe/Kiev", "Europe/Kirov", "Europe/Kyiv",
	"Europe/Lisbon", "Europe/Ljubljana", "Europe/London", "Europe/Luxembourg",
	"Europe/Madrid", "Europe/Malta", "Europe/Mariehamn", "Europe/Minsk",
	"Europe/Monaco", "Europe/Moscow", "Europe/Nicosia", "Europe/Oslo",
	"Europe/Paris", "Europe/Podgorica", "Europe/Prague", "Europe/Riga",
	"Europe/Rome", "Europe/Samara", "Europe/San_Marino", "Europe/Sarajevo",
	"Europe/Saratov", "Europe/Simferopol", "Europe/Skopje", "Europe/Sofia",
	"Europe/Stockholm", "Europe/Tallinn", "Europe/Tirane", "Europe/Ulyanovsk",
	"Europe/Uzhgorod", "Europe/Vaduz", "Europe/Vatican", "Europe/Vienna",
	"Europe/Vilnius", "Europe/Volgograd", "Europe/Warsaw", "Europe/Zagreb",
	"Europe/Zaporozhye", "Europe/Zurich",
	"Indian/Antananarivo", "Indian/Chagos", "Indian/Christmas", "Indian/Cocos",
	"Indian/Comoro", "Indian/Kerguelen", "Indian/Mahe", "Indian/Maldives",
	"Indian/Mauritius", "Indian/Mayotte", "Indian/Reunion",
	"Pacific/Apia", "Pacific/Auckland", "Pacific/Bougainville", "Pacific/Chatham",
	"Pacific/Chuuk", "Pacific/Easter", "Pacific/Efate", "Pacific/Fakaofo",
	"Pacific/Fiji", "Pacific/Funafuti", "Pacific/Galapagos", "Pacific/Gambier",
	"Pacific/Guadalcanal", "Pacific/Guam", "Pacific/Honolulu", "Pacific/Kanton",
	"Pacific/Kiritimati", "Pacific/Kosrae", "Pacific/Kwajalein", "Pacific/Majuro",
	"Pacific/Marquesas", "Pacific/Midway", "Pacific/Nauru", "Pacific/Niue",
	"Pacific/Norfolk", "Pacific/Noumea", "Pacific/Pago_Pago", "Pacific/Palau",
	"Pacific/Pitcairn", "Pacific/Pohnpei", "Pacific/Port_Moresby", "Pacific/Rarotonga",
	"Pacific/Saipan", "Pacific/Tahiti", "Pacific/Tarawa", "Pacific/Tongatapu",
	"Pacific/Wake", "Pacific/Wallis",
	"UTC",
}

func (s *APIServer) handleSwagger(w http.ResponseWriter, r *http.Request) {
	swagger := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "TaskPilot API",
			"description": "REST API for managing scheduled jobs and execution history",
			"version":     "1.0.0",
		},
		"servers": []map[string]interface{}{
			{
				"url":         fmt.Sprintf("http://localhost:%d", s.port),
				"description": "Local server",
			},
		},
		"paths": map[string]interface{}{
			"/api/jobs": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List all jobs",
					"description": "Get a list of all scheduled jobs",
					"tags":        []string{"jobs"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/Job",
										},
									},
								},
							},
						},
					},
				},
				"post": map[string]interface{}{
					"summary":     "Create a new job",
					"description": "Create a new scheduled job",
					"tags":        []string{"jobs"},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/Job",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Job",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad Request",
						},
					},
				},
			},
			"/api/jobs/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get job by ID",
					"description": "Get details of a specific job",
					"tags":        []string{"jobs"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Job ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Job",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Not Found",
						},
					},
				},
				"put": map[string]interface{}{
					"summary":     "Update a job",
					"description": "Update an existing job by ID",
					"tags":        []string{"jobs"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Job ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/Job",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Job",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad Request",
						},
						"404": map[string]interface{}{
							"description": "Not Found",
						},
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Delete a job",
					"description": "Delete a job by ID (cascade deletes history)",
					"tags":        []string{"jobs"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Job ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "No Content",
						},
						"404": map[string]interface{}{
							"description": "Not Found",
						},
					},
				},
			},
			"/api/jobs/{id}/history": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get history for a specific job",
					"description": "Get all history records for a specific job with optional ordering",
					"tags":        []string{"history"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Job ID (UUID format)",
							"schema": map[string]interface{}{
								"type":   "string",
								"format": "uuid",
							},
						},
						{
							"name":        "order",
							"in":          "query",
							"required":    false,
							"description": "Order by timestamp",
							"schema": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"asc", "desc"},
								"default": "desc",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/History",
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad Request - Invalid job ID format",
						},
						"404": map[string]interface{}{
							"description": "Not Found - Job does not exist",
						},
					},
				},
			},
			"/api/history": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List all history records",
					"description": "Get a list of all job execution history records with optional filtering and ordering",
					"tags":        []string{"history"},
					"parameters": []map[string]interface{}{
						{
							"name":        "job_id",
							"in":          "query",
							"required":    false,
							"description": "Filter by job ID (UUID format)",
							"schema": map[string]interface{}{
								"type":   "string",
								"format": "uuid",
							},
						},
						{
							"name":        "order",
							"in":          "query",
							"required":    false,
							"description": "Order by timestamp",
							"schema": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"asc", "desc"},
								"default": "desc",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/History",
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "Bad Request - Invalid query parameter",
						},
					},
				},
			},
			"/api/history/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get history record by ID",
					"description": "Get details of a specific history record",
					"tags":        []string{"history"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "History ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/History",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Not Found",
						},
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Delete history record",
					"description": "Delete a specific history record by ID",
					"tags":        []string{"history"},
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "History ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{
							"description": "No Content",
						},
						"404": map[string]interface{}{
							"description": "Not Found",
						},
					},
				},
			},
			"/api/system/time": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get system time and timezone",
					"description": "Returns the current server time in multiple formats along with timezone information",
					"tags":        []string{"system"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/SystemTime",
									},
									"example": map[string]interface{}{
										"timestamp":          1735689600,
										"iso8601":            "2025-01-01T00:00:00Z",
										"timezone":           "America/Los_Angeles",
										"timezone_offset":    "-08:00",
										"utc_offset_seconds": -28800,
									},
								},
							},
						},
					},
				},
			},
			"/api/system/timezones": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List available timezones",
					"description": "Returns the server's current IANA timezone and a sorted list of all well-known IANA timezone names",
					"tags":        []string{"system"},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Timezones",
									},
									"example": map[string]interface{}{
										"current":   "America/Los_Angeles",
										"timezones": []string{"Africa/Abidjan", "America/Los_Angeles", "Asia/Tokyo", "Europe/London", "UTC"},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Job": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":                map[string]interface{}{"type": "string"},
						"name":              map[string]interface{}{"type": "string"},
						"command":           map[string]interface{}{"type": "string"},
						"directory":         map[string]interface{}{"type": "string"},
						"schedule":          map[string]interface{}{"type": "string"},
						"sound_file":        map[string]interface{}{"type": "string"},
						"on_success_cmd":    map[string]interface{}{"type": "string"},
						"last_result":       map[string]interface{}{"type": "string"},
						"status":            map[string]interface{}{"type": "string"},
						"schedule_type":     map[string]interface{}{"type": "string"},
						"paused":            map[string]interface{}{"type": "boolean"},
						"run_at":            map[string]interface{}{"type": "integer", "format": "int64"},
						"delay_minutes":     map[string]interface{}{"type": "integer"},
						"last_run_at":       map[string]interface{}{"type": "integer", "format": "int64"},
						"next_run_at":       map[string]interface{}{"type": "integer", "format": "int64"},
						"last_scheduled_at": map[string]interface{}{"type": "integer", "format": "int64"},
					},
					"required": []string{"name", "command", "schedule"},
				},
				"History": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":           map[string]interface{}{"type": "string"},
						"job_id":       map[string]interface{}{"type": "string"},
						"output":       map[string]interface{}{"type": "string"},
						"exit_code":    map[string]interface{}{"type": "integer"},
						"timestamp":    map[string]interface{}{"type": "integer", "format": "int64"},
						"duration_ms":  map[string]interface{}{"type": "integer", "format": "int64"},
						"scheduled_at": map[string]interface{}{"type": "integer", "format": "int64"},
						"trigger_type": map[string]interface{}{"type": "string"},
					},
				},
				"SystemTime": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"timestamp":          map[string]interface{}{"type": "integer", "format": "int64", "description": "Unix timestamp (seconds since epoch)"},
						"iso8601":            map[string]interface{}{"type": "string", "description": "ISO8601 formatted time string (RFC3339)"},
						"timezone":           map[string]interface{}{"type": "string", "description": "IANA timezone name (e.g., America/Los_Angeles, UTC)"},
						"timezone_offset":    map[string]interface{}{"type": "string", "description": "Timezone offset from UTC (e.g., -08:00, +05:30)"},
						"utc_offset_seconds": map[string]interface{}{"type": "integer", "description": "UTC offset in seconds"},
					},
				},
				"Timezones": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"current":   map[string]interface{}{"type": "string", "description": "IANA name of the server's local timezone"},
						"timezones": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Sorted list of all well-known IANA timezone names"},
					},
				},
			},
		},
	}

	s.sendJSON(w, swagger, http.StatusOK)
}

// sendJSON sends a JSON response
func (s *APIServer) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// sendError sends an error response
func (s *APIServer) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}

// handleMCP routes MCP requests - GET for SSE connection, POST for JSON-RPC requests
func (s *APIServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleMCPSSE(w, r)
	case http.MethodPost:
		s.handleMCPRequest(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMCPSSE establishes an SSE connection for streaming events to the client
func (s *APIServer) handleMCPSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Generate unique client ID
	clientID := uuid.New().String()

	// Register client and get event channel
	eventChan := s.mcpServer.RegisterClient(clientID)
	defer s.mcpServer.UnregisterClient(clientID)

	// Send connection acknowledgment
	connAck := MCPEvent{
		Type: "connection.established",
		Data: map[string]string{
			"client_id": clientID,
			"status":    "connected",
		},
	}
	s.writeSSEEvent(w, connAck)

	// Flush to send the connection acknowledgment immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Create ticker for heartbeat
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	// Stream events to client
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed, client disconnected
				return
			}
			s.writeSSEEvent(w, event)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			heartbeat := MCPEvent{
				Type: "heartbeat",
				Data: map[string]int64{
					"timestamp": time.Now().Unix(),
				},
			}
			s.writeSSEEvent(w, heartbeat)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-r.Context().Done():
			// Client disconnected
			log.Printf("MCP client disconnected: %s", clientID)
			return
		}
	}
}

// writeSSEEvent writes an event in SSE format
func (s *APIServer) writeSSEEvent(w http.ResponseWriter, event MCPEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Error marshaling SSE event: %v", err)
		return
	}

	// SSE format: "data: <json>\n\n"
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// handleMCPRequest processes a JSON-RPC request and returns the response
func (s *APIServer) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	var req MCPRequest

	// Parse JSON-RPC request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response := MCPResponse{
			JSONRPC: "2.0",
			Error: &MCPError{
				Code:    MCPErrorParseError,
				Message: "Parse error: " + err.Error(),
			},
		}
		s.sendJSON(w, response, http.StatusOK)
		return
	}

	// Process request
	response := s.mcpServer.HandleRequest(r.Context(), req)

	// Only send response if it's not empty (notifications return empty response)
	if response.JSONRPC != "" {
		s.sendJSON(w, response, http.StatusOK)
	} else {
		// For notifications, return 204 No Content
		w.WriteHeader(http.StatusNoContent)
	}
}
