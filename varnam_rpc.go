package main

import "log"

type Args struct {
	LangCode string
	Word     string
}

type VarnamRPC struct{}

func (v *VarnamRPC) Learn(args *Args, reply *bool) error {
	log.Println("From RPC Learn")
	return nil
}
