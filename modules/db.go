package modules

import (
	"database/sql"
	"encoding/json"
	"log"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	DB     *sql.DB
	Crypto *Crypto
	Secret string
}

type State struct {
	ID     string
	UserID string
	Value  string
}

func NewDB(path string, secret string) *DB {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Fatal(err)
	}

	db.Exec("CREATE TABLE IF NOT EXISTS `states` (id varchar, user_id varchar, value text)")

	return &DB{
		DB:     db,
		Crypto: NewCrypto([]byte(secret)),
	}
}

func (d *DB) Close() {
	d.DB.Close()
}

func (d *DB) GetSates() (result []State) {
	rows, err := d.DB.Query("SELECT id, user_id, value FROM `states`")

	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var state State
		if err := rows.Scan(&state.ID, &state.UserID, &state.Value); err != nil {
			continue
		}

		v, _ := d.Crypto.Decrypt(state.Value)

		if v != nil {
			state.Value = *v
		}

		result = append(result, state)
	}

	return
}

func (d *DB) CreateUserState(userID string, value any) string {
	ID := uuid.New().String()
	body, _ := json.Marshal(value)

	// handle encrypt
	v, _ := d.Crypto.Encrypt(string(body))

	d.DB.Exec("INSERT INTO `states`(id, user_id, value) VALUES(?, ?, ?)", ID, userID, *v)

	return ID
}

func (d *DB) DropUserState(userID string, ID string) error {
	_, err := d.DB.Exec("DELETE FROM `states` WHERE user_id=? AND id=?", userID, ID)

	return err
}

func (d *DB) DropUserStates(userID string) error {
	_, err := d.DB.Exec("DELETE FROM `states` WHERE user_id=", userID)

	return err
}

func (d *DB) GetUserSates(userID string) (result []State) {
	rows, err := d.DB.Query("SELECT id, user_id, value FROM `states` WHERE user_id=?", userID)

	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var state State

		if err := rows.Scan(&state.ID, &state.UserID, &state.Value); err != nil {
			continue
		}

		v, _ := d.Crypto.Decrypt(state.Value)

		if v != nil {
			state.Value = *v
		}

		result = append(result, state)
	}

	return
}
