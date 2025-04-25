package db

import (
	"database/sql"
	"errors"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type DatabaseStruct struct {
	*sql.DB
}

func InitDB(dbPath string) (*DatabaseStruct, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Could not open DB file: %v", err)
		return nil, err
	}

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			quantity INTEGER NOT NULL,
			unit_price_value INTEGER NOT NULL,
			unit_price_currency TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			total_value INTEGER NOT NULL,
			total_currency TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS order_items (
			order_id TEXT,
			item_id TEXT,
			quantity INTEGER NOT NULL,
			PRIMARY KEY (order_id, item_id),
			FOREIGN KEY (order_id) REFERENCES orders(id),
			FOREIGN KEY (item_id) REFERENCES items(id)
		);
		CREATE TABLE IF NOT EXISTS shipments (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			status TEXT NOT NULL,
			tracking_number TEXT,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY (order_id) REFERENCES orders(id)
		);
		CREATE TABLE IF NOT EXISTS users (
			api_key TEXT PRIMARY KEY,
			role TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_key TEXT NOT NULL,
			method TEXT NOT NULL,
			request_data TEXT NOT NULL,
			status TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			FOREIGN KEY (api_key) REFERENCES users(api_key)
		);
	`)

	if err != nil {
		log.Println("Error inserting tables")
		return nil, err
	}

	// insert default users for testing
	_, err = db.Exec(`
		INSERT OR IGNORE INTO USERS (api_key, role) VALUES
		('customer-key-123', 'customer'),
		('admin-key-456', 'admin'),
		('admin-key-789', 'admin')
	`)
	if err != nil {
		log.Println("Error insert default values to users")
		return nil, err
	}

	return &DatabaseStruct{db}, nil
}

func (db *DatabaseStruct) ValidateAPIKey(apiKey string) (string, error) {
	var role string
	err := db.QueryRow("SELECT role FROM users WHERE api_key = ?", apiKey).Scan(&role)
	if err == sql.ErrNoRows {
		return "", errors.New("Invalid API Key")
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           int64
	APIKey       string
	Method       string
	RequestData  string
	Status       string
	Timestamp    int64
}

func (db *DatabaseStruct) GetAuditLogs(apiKey string, limit, offset int) ([]*AuditLog, error) {
	rows, err := db.Query(`
		SELECT id, api_key, method, request_data, status, timestamp
		FROM audit_logs
		WHERE api_key = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, apiKey, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		log := &AuditLog{}
		if err := rows.Scan(&log.ID, &log.APIKey, &log.Method, &log.RequestData, &log.Status, &log.Timestamp); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}