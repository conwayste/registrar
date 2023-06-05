package api

import (
	"testing"
)

func TestValidHostAndPort(t *testing.T) {
	if !validHostAndPort("myserver.example.com:2016") {
		t.Error("ruh roh")
	}
	if validHostAndPort("myserver.example.com") {
		t.Error("ruh roh")
	}
	if validHostAndPort("") {
		t.Error("ruh roh")
	}
}
