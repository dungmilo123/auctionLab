package store

import "os"

const defaultPath = "./auction.db"

// PathFromEnv resolves the database path: DB_PATH when set, else ./auction.db.
// The override exists so tests and local dev get an isolated database.
func PathFromEnv() string {
	if path := os.Getenv("DB_PATH"); path != "" {
		return path
	}
	return defaultPath
}
