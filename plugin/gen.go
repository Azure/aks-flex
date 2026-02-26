package main

//go:generate sh -c "cd .. && find plugin -name '*.proto' | xargs protoc --go_opt=paths=source_relative --go_out=. --go-grpc_opt=paths=source_relative --go-grpc_out=."
