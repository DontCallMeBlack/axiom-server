package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ==========================================
// MASTER BOSS PAYOUT DATABASE
// ==========================================

var bossPayouts = map[string]int{
	// EGB (Epic Game Bosses) - 6* Values
	"Aggy":     5,
	"Hrung":    5,
	"Mordi":    10,
	"Necro":    20,
	"Base":     20,
	"Prime":    25,
	"Gele":     50,
	"Dino":     65,
	"Factions": 20,
	"Crom":     120,

	// Bosses with distinct 5* and 6* variants
	"BT_5": 4,
	"BT_6": 30,

	// Valley Bosses
	"Valley_Extra": 20, // Tree, Dragon, Bats
	"Valley_Aco":   10, // Acolyte, Doom

	// Legacy Bosses
	"Osan_5":      5,
	"Osan_6":      10,
	"Spider_5":    5,
	"Spider_6":    10,
	"Mono_Obby_5": 5,
	"Mono_Obby_6": 10,

	// DL (Dragon Lord)
	"DL_155_4": 0, "DL_155_5": 0, "DL_155_6": 0,
	"DL_160_4": 0, "DL_160_5": 2, "DL_160_6": 3,
	"DL_165_4": 0, "DL_165_5": 2, "DL_165_6": 3,
	"DL_170_4": 0, "DL_170_5": 6, "DL_170_6": 8,
	"DL_180_4": 1, "DL_180_5": 6, "DL_180_6": 8,

	// EDL (Exalted Dragon Lord)
	"EDL_185_4": 0, "EDL_185_5": 2, "EDL_185_6": 4,
	"EDL_190_4": 0, "EDL_190_5": 0, "EDL_190_6": 0,
	"EDL_195_4": 0, "EDL_195_5": 2, "EDL_195_6": 4,
	"EDL_200_4": 0, "EDL_200_5": 2, "EDL_200_6": 4,
	"EDL_205_4": 0, "EDL_205_5": 2, "EDL_205_6": 4,
	"EDL_210_4": 6, "EDL_210_5": 8, "EDL_210_6": 10,
	"EDL_215_4": 6, "EDL_215_5": 8, "EDL_215_6": 10,
}

// ==========================================
// 1. DATA STRUCTURES
// ==========================================
type LogKillRequest struct {
	BossFormat string   `json:"boss_format"` // e.g., "/gele" or "210.5"
	BossID     string   `json:"boss_id"`     // Database key e.g., "Gele" or "EDL_210_5"
	Attendees  []string `json:"attendees"`
	LoggedBy   string   `json:"logged_by"`
}

type BossTimer struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	WindowEnds time.Time `json:"window_ends_at"`
}

type RegisterRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CharName  string `json:"char_name"`
	GameClass string `json:"game_class"`
}

type GatekeeperRequest struct {
	RecruitID   string `json:"recruit_id"`
	OfficerID   string `json:"officer_id"`
	OfficerRole string `json:"officer_role"`
	Action      string `json:"action"`
}

type ApprovalRequest struct {
	LogID     string `json:"log_id"`
	OfficerID string `json:"officer_id"`
	Action    string `json:"action"`
}

type BidRequest struct {
	AuctionID string `json:"auction_id"`
	CharName  string `json:"char_name"`
	BidAmount int    `json:"bid_amount"`
}

type Auction struct {
	ID               string    `json:"id"`
	ItemName         string    `json:"item_name"`
	ImageURL         string    `json:"image_url"`
	HolderName       string    `json:"holder_name"`
	ClassRestriction string    `json:"class_restriction"`
	EndTime          time.Time `json:"end_time"`
	HighestBid       int       `json:"highest_bid"`
	HighestBidder    string    `json:"highest_bidder"`
	IsActive         bool      `json:"is_active"`
}

type StartAuctionRequest struct {
	ItemName         string `json:"item_name"`
	ImageURL         string `json:"image_url"`
	HolderName       string `json:"holder_name"`
	ClassRestriction string `json:"class_restriction"`
}

type LootItem struct {
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

// ==========================================
// MOCK DATABASE STATE
// ==========================================

var (
	// Store ALL active and historical auctions
	mockAuctions = map[string]*Auction{
		"auc-1": {
			ID:               "auc-1",
			ItemName:         "Godly Earthbark Helm",
			ImageURL:         "/loot-images/Earthbark_Helm.jpg", // Ensure you have a test image or it will break
			HolderName:       "Chief",
			ClassRestriction: "druid",
			EndTime:          time.Now().UTC().Add(3 * time.Minute),
			HighestBid:       120,
			HighestBidder:    "NatureHeals",
			IsActive:         true,
		},
	}

	mockEscrow = map[string]int{
		"NatureHeals": 120,
	}

	mockRoster = map[string]string{
		"ShieldWall":  "warrior",
		"NatureHeals": "druid",
		"ArcaneFire":  "mage",
	}
)

// Global state mutex to protect in-memory maps for transactional updates
var stateMu sync.Mutex

// Ledger record for accounting
type LedgerRecord struct {
	ID        string    `json:"id"`
	CharName  string    `json:"char_name"`
	Amount    int       `json:"amount"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	AuctionID string    `json:"auction_id,omitempty"`
}

var ledgerRecords = make([]LedgerRecord, 0)

func addLedgerRecord(r LedgerRecord) {
	ledgerRecords = append(ledgerRecords, r)
	if dbEnabled && ledgerColl != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := ledgerColl.InsertOne(ctx, r); err != nil {
			log.Printf("Failed to persist ledger record: %v", err)
		}
	}
}

// MongoDB integration (optional)
var dbClient *mongo.Client
var dbEnabled bool
var userColl *mongo.Collection
var ledgerColl *mongo.Collection

// initMongo attempts to connect to MongoDB when MONGO_URI is set.
// On success it loads users and ledger into in-memory structures to keep behavior unchanged.
func initMongo() {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Println("MONGO_URI not set; using in-memory stores")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Printf("Mongo connect error: %v", err)
		return
	}
	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("Mongo ping failed: %v", err)
		return
	}

	dbClient = client
	dbEnabled = true
	db := client.Database("axiom")
	userColl = db.Collection("users")
	ledgerColl = db.Collection("ledger")

	log.Println("Connected to MongoDB — loading users and ledger into memory")

	// Load users
	cur, err := userColl.Find(ctx, bson.D{})
	if err == nil {
		defer cur.Close(ctx)
		for cur.Next(ctx) {
			var u User
			if err := cur.Decode(&u); err == nil {
				users[u.Username] = &u
				if u.CharName != "" {
					mockRoster[u.CharName] = u.GameClass
					if _, ok := mockEscrow[u.CharName]; !ok {
						mockEscrow[u.CharName] = 0
					}
				}
			}
		}
	} else {
		log.Printf("Failed to load users from MongoDB: %v", err)
	}

	// Load ledger
	cur2, err2 := ledgerColl.Find(ctx, bson.D{})
	if err2 == nil {
		defer cur2.Close(ctx)
		for cur2.Next(ctx) {
			var lr LedgerRecord
			if err := cur2.Decode(&lr); err == nil {
				ledgerRecords = append(ledgerRecords, lr)
			}
		}
	} else {
		log.Printf("Failed to load ledger from MongoDB: %v", err2)
	}
}

// Simple in-memory user store for demo purposes
type User struct {
	Username  string `json:"username"`
	Password  string `json:"-"`
	CharName  string `json:"char_name"`
	GameClass string `json:"game_class"`
	Role      string `json:"role"`
}

var users = map[string]*User{}

// ==========================================
// 2. ROLLING ACTIVITY LOGIC
// ==========================================

func checkBiddingEligibility(charName string) bool {
	now := time.Now().UTC()
	endOfYesterday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startOfWindow := endOfYesterday.AddDate(0, 0, -7)

	fmt.Printf("[SYSTEM] Checking activity for account of '%s'\n", charName)
	fmt.Printf("         Window: %s to %s (UTC)\n", startOfWindow.Format("Jan 02, 15:04"), endOfYesterday.Format("Jan 02, 15:04"))

	mockMainActivity := 150
	mockAltActivity := 200
	totalAccountActivity := mockMainActivity + mockAltActivity

	return totalAccountActivity >= 300
}

// ==========================================
// 3. WEBSOCKET ENGINE
// ==========================================

type Hub struct {
	sync.RWMutex
	clients   map[*websocket.Conn]bool
	broadcast chan []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Hub) Run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message := <-h.broadcast:
			h.RLock()
			for client := range h.clients {
				if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
					client.Close()
					delete(h.clients, client)
				}
			}
			h.RUnlock()

		case <-ticker.C:
			// Ping for alive connections
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	h.Lock()
	h.clients[conn] = true
	h.Unlock()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			h.Lock()
			delete(h.clients, conn)
			h.Unlock()
			conn.Close()
			break
		}
	}
}

// finalizeDueAuctions finalizes auctions whose EndTime has passed.
// It performs updates under stateMu to simulate transactional escrow/settlement.
func finalizeDueAuctions(h *Hub) {
	now := time.Now().UTC()
	stateMu.Lock()
	defer stateMu.Unlock()

	for _, auction := range mockAuctions {
		if !auction.IsActive {
			continue
		}
		if now.After(auction.EndTime) || now.Equal(auction.EndTime) {
			auction.IsActive = false

			// Settle escrow: winner pays their highest bid (remove from escrow)
			winner := auction.HighestBidder
			amt := auction.HighestBid
			if winner != "" && winner != "None" {
				currentEsc := mockEscrow[winner]
				pay := amt
				if pay > currentEsc {
					pay = currentEsc
				}
				mockEscrow[winner] = currentEsc - pay

				// ledger entry: spend by winner
				addLedgerRecord(LedgerRecord{
					ID:        fmt.Sprintf("led-%d", time.Now().UnixNano()),
					CharName:  winner,
					Amount:    -pay,
					Type:      "auction_spend",
					Timestamp: now,
					AuctionID: auction.ID,
				})
			}

			// Broadcast auction finalization
			if h != nil {
				payload, _ := json.Marshal(map[string]interface{}{
					"type":       "auction_finalized",
					"auction_id": auction.ID,
					"winner":     auction.HighestBidder,
					"amount":     auction.HighestBid,
				})
				h.broadcast <- payload
			}
		}
	}
}

// runAuctionFinalizer runs a periodic finalizer that finalizes due auctions.
func runAuctionFinalizer(h *Hub, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			finalizeDueAuctions(h)
		case <-stop:
			return
		}
	}
}

// ==========================================
// 4. API ENDPOINTS
// ==========================================

func handlePlaceBid(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BidRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")

		stateMu.Lock()
		auction, exists := mockAuctions[req.AuctionID]
		if !exists || !auction.IsActive {
			stateMu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Auction not found or closed."})
			return
		}

		now := time.Now().UTC()

		if now.After(auction.EndTime) {
			stateMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Auction has ended."})
			return
		}

		playerClass, charExists := mockRoster[req.CharName]
		if !charExists {
			stateMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Character not found in the database roster."})
			return
		}

		if auction.ClassRestriction != "all" && playerClass != auction.ClassRestriction {
			stateMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			errorMsg := fmt.Sprintf("Class mismatch! %s is a %s, but this item is restricted to %s.", req.CharName, strings.ToUpper(playerClass), strings.ToUpper(auction.ClassRestriction))
			json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
			return
		}

		if !checkBiddingEligibility(req.CharName) {
			stateMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bidding Locked: Under 300 DKP pooled activity in the last 7 days."})
			return
		}

		playerTotalBank := 450
		currentEscrow := mockEscrow[req.CharName]
		availableDKP := playerTotalBank - currentEscrow

		bidDifference := req.BidAmount
		if auction.HighestBidder == req.CharName {
			bidDifference = req.BidAmount - auction.HighestBid
		}

		if bidDifference > availableDKP {
			stateMu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Insufficient DKP. Bank: %d, In Escrow: %d, Available: %d", playerTotalBank, currentEscrow, availableDKP),
			})
			return
		}

		if req.BidAmount <= auction.HighestBid {
			stateMu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Bid must be higher than current bid."})
			return
		}

		timeRemaining := auction.EndTime.Sub(now)
		extensionTriggered := false
		if timeRemaining <= 5*time.Minute {
			auction.EndTime = auction.EndTime.Add(5 * time.Minute)
			extensionTriggered = true
			fmt.Printf("[AUCTION] Anti-snipe triggered! Auction extended by 5 minutes. New End Time: %s\n", auction.EndTime.Format("15:04:05"))
		}

		if auction.HighestBidder != "" && auction.HighestBidder != req.CharName && auction.HighestBidder != "None" {
			mockEscrow[auction.HighestBidder] -= auction.HighestBid
			if mockEscrow[auction.HighestBidder] < 0 {
				mockEscrow[auction.HighestBidder] = 0
			}
		}

		auction.HighestBid = req.BidAmount
		auction.HighestBidder = req.CharName
		mockEscrow[req.CharName] += bidDifference

		fmt.Printf("[AUCTION] %s successfully bid %d DKP on %s.\n", req.CharName, req.BidAmount, auction.ItemName)

		stateMu.Unlock()

		bidPayload, _ := json.Marshal(map[string]interface{}{
			"type":       "new_bid",
			"auction_id": auction.ID,
			"char_name":  req.CharName,
			"amount":     req.BidAmount,
			"extended":   extensionTriggered,
		})
		hub.broadcast <- bidPayload

		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

func handleStartAuction(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req StartAuctionRequest
		json.NewDecoder(r.Body).Decode(&req)

		newID := fmt.Sprintf("auc-%d", time.Now().Unix())

		newAuction := &Auction{
			ID:               newID,
			ItemName:         req.ItemName,
			ImageURL:         req.ImageURL,
			HolderName:       req.HolderName,
			ClassRestriction: req.ClassRestriction,
			EndTime:          time.Now().UTC().Add(24 * time.Hour),
			HighestBid:       0,
			HighestBidder:    "None",
			IsActive:         true,
		}

		mockAuctions[newID] = newAuction

		fmt.Printf("[AUCTION] %s posted a new drop: %s\n", req.HolderName, req.ItemName)

		payload, _ := json.Marshal(map[string]interface{}{
			"type":    "auction_started",
			"auction": newAuction,
		})
		hub.broadcast <- payload

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

// UPDATED: Return an array of ALL active auctions
func handleGetActiveAuctions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var activeAuctions []*Auction

	for _, auc := range mockAuctions {
		if auc.IsActive {
			activeAuctions = append(activeAuctions, auc)
		}
	}

	json.NewEncoder(w).Encode(activeAuctions)
}

func handleGetLoot(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir("./loot-images")

	// Force it to initialize as an empty array rather than a nil map
	loot := make([]LootItem, 0)

	if err == nil {
		for _, file := range files {
			if !file.IsDir() {
				fileName := file.Name()
				ext := filepath.Ext(fileName)
				cleanName := strings.ReplaceAll(strings.TrimSuffix(fileName, ext), "_", " ")

				loot = append(loot, LootItem{
					Name:     cleanName,
					ImageURL: "/loot-images/" + fileName,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loot)
}

func handleGatekeeper(w http.ResponseWriter, r *http.Request) {}
func handleApprovalQueue(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {}
}
func handleRevertTransaction(w http.ResponseWriter, r *http.Request) {}

type LedgerEntry struct {
	Role           string `json:"role"`
	CharName       string `json:"char_name"`
	GameClass      string `json:"game_class"`
	PooledActivity int    `json:"pooled_activity"`
	TotalBank      int    `json:"total_bank"`
	InEscrow       int    `json:"in_escrow"`
	AvailableDKP   int    `json:"available_dkp"`
}

// Deterministic mock of pooled 7-day activity per character for ledger display
func pooledActivityFor(name string) int {
	sum := 0
	for i := 0; i < len(name); i++ {
		sum += int(name[i])
	}
	// Map sum into a reasonable DKP activity number
	return 100 + (sum % 400)
}

// handleGetLedger returns a list of ledger entries built from users, roster, and escrow
func handleGetLedger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	entries := make([]LedgerEntry, 0)

	// Collect all character names from mockRoster (ensures we include non-user roster entries)
	for charName, class := range mockRoster {
		totalBank := 450
		inEscrow := mockEscrow[charName]
		available := totalBank - inEscrow

		// Try to find role from users map by matching CharName
		role := "Clansman"
		for _, u := range users {
			if u.CharName == charName {
				if u.Role != "" {
					role = u.Role
				}
				break
			}
		}

		pa := pooledActivityFor(charName)

		entries = append(entries, LedgerEntry{
			Role:           role,
			CharName:       charName,
			GameClass:      class,
			PooledActivity: pa,
			TotalBank:      totalBank,
			InEscrow:       inEscrow,
			AvailableDKP:   available,
		})
	}

	// Also include any users that might not be in mockRoster yet
	for _, u := range users {
		if _, exists := mockRoster[u.CharName]; !exists {
			totalBank := 450
			inEscrow := mockEscrow[u.CharName]
			available := totalBank - inEscrow
			entries = append(entries, LedgerEntry{
				Role:           u.Role,
				CharName:       u.CharName,
				GameClass:      u.GameClass,
				PooledActivity: pooledActivityFor(u.CharName),
				TotalBank:      totalBank,
				InEscrow:       inEscrow,
				AvailableDKP:   available,
			})
		}
	}

	// Sort by Role (alphabetically) then by AvailableDKP descending
	// Use simple stable sort
	// First sort by AvailableDKP descending
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].AvailableDKP > entries[j].AvailableDKP
	})
	// Then sort by Role ascending
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Role) < strings.ToLower(entries[j].Role)
	})

	json.NewEncoder(w).Encode(entries)
}

// Simple signup: stores user in memory and adds to roster for demo/testing
func handleSignup(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
		return
	}

	if req.Username == "" || req.CharName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "username and char_name required"})
		return
	}

	if _, exists := users[req.Username]; exists {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "user already exists"})
		return
	}

	u := &User{
		Username:  req.Username,
		Password:  req.Password, // NOTE: plain text for demo only
		CharName:  req.CharName,
		GameClass: req.GameClass,
		Role:      "member",
	}

	users[req.Username] = u
	// Add to roster so newly created users can bid / appear in ledger
	mockRoster[req.CharName] = req.GameClass
	mockEscrow[req.CharName] = 0

	if dbEnabled && userColl != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := userColl.InsertOne(ctx, u); err != nil {
			log.Printf("Failed to persist user to MongoDB: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// /api/me -- simple demo endpoint that returns a placeholder chief account if present
func handleGetMe(w http.ResponseWriter, r *http.Request) {
	// For demo, prefer query param ?user=... otherwise default to 'chief'
	q := r.URL.Query().Get("user")
	var u *User
	if q != "" {
		u = users[q]
	} else {
		u = users["chief"]
	}

	if u == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no user found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Return only public fields useful to the UI
	json.NewEncoder(w).Encode(map[string]string{
		"username":   u.Username,
		"char_name":  u.CharName,
		"game_class": u.GameClass,
		"role":       u.Role,
	})
}

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}
func handleLogKill(w http.ResponseWriter, r *http.Request) {
	var req LogKillRequest
	json.NewDecoder(r.Body).Decode(&req)

	// Look up the DKP value from the Master Database you added earlier
	dkpValue := bossPayouts[req.BossID]

	fmt.Printf("[APPROVAL QUEUE] %s logged %s (%s). Worth %d DKP.\n", req.LoggedBy, req.BossFormat, req.BossID, dkpValue)
	fmt.Printf("Attendees (%d): %v\n", len(req.Attendees), req.Attendees)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Sent to Officer Queue"})
}

func main() {
	fmt.Println("Booting Axiom Command Center Backend...")

	// Attempt to initialize MongoDB (optional)
	initMongo()

	hub := NewHub()
	go hub.Run()
	// start auction finalizer (runs every 30 seconds)
	stopFinalizer := make(chan struct{})
	go runAuctionFinalizer(hub, 30*time.Second, stopFinalizer)

	// Create a placeholder chief account for visualization/testing (only if not loaded from DB)
	if _, ok := users["chief"]; !ok {
		users["chief"] = &User{
			Username:  "chief",
			Password:  "chiefpass",
			CharName:  "Chief",
			GameClass: "officer",
			Role:      "chief",
		}
		// If DB is enabled, persist chief as well
		if dbEnabled && userColl != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := userColl.InsertOne(ctx, users["chief"]); err != nil {
				log.Printf("Failed to persist chief user: %v", err)
			}
		}
	}
	// Ensure chief appears in the roster and escrow maps
	mockRoster["Chief"] = "officer"
	if _, ok := mockEscrow["Chief"]; !ok {
		mockEscrow["Chief"] = 0
	}

	http.HandleFunc("/ws", hub.ServeWS)
	http.HandleFunc("/api/signup", enableCORS(handleSignup))
	http.HandleFunc("/api/gatekeeper", enableCORS(handleGatekeeper))
	http.HandleFunc("/api/approve", enableCORS(handleApprovalQueue(hub)))
	http.HandleFunc("/api/revert", enableCORS(handleRevertTransaction))
	http.HandleFunc("/api/log_kill", enableCORS(handleLogKill))
	http.HandleFunc("/api/me", enableCORS(handleGetMe))

	http.HandleFunc("/api/bid", enableCORS(handlePlaceBid(hub)))
	http.HandleFunc("/api/loot", enableCORS(handleGetLoot))
	http.HandleFunc("/api/ledger", enableCORS(handleGetLedger))
	http.HandleFunc("/api/auctions/active", enableCORS(handleGetActiveAuctions)) // UPDATED ROUTE
	http.HandleFunc("/api/auction/start", enableCORS(handleStartAuction(hub)))

	os.MkdirAll("./loot-images", os.ModePerm)
	fs := http.FileServer(http.Dir("./loot-images"))
	http.Handle("/loot-images/", http.StripPrefix("/loot-images/", fs))

	frontendFS := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", frontendFS)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	fmt.Printf("Server running on http://localhost:%s\n", port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("Server crashed: ", err)
	}
}
