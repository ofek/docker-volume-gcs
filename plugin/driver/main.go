package main

import (
	"os/user"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

func main() {
	u, _ := user.Lookup("root")
	gid, _ := strconv.Atoi(u.Gid)

	driver := NewGCSDriver()
	handler := volume.NewHandler(driver)
	if err := handler.ServeUnix(PluginName, gid); err != nil {
		log.Fatalf("Error serving socket %s.sock: %s", PluginName, err)
	}
}
