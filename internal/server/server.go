package server

import db "github.com/skynicklaus/ecommerce-api/db/sqlc"

type Server struct {
	port  int
	store db.Store
}

func NewServer() {
}
