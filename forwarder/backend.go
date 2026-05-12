package main

import (
	"net/url"
)

type Backend struct {
	// Name reprensents the name of the backend which is used to [Maglev]
	Name string 
	Address *url.URL
}

