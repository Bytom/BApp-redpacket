package main

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/redpacket/server/config"
	"github.com/redpacket/server/database"
	"github.com/redpacket/server/synchron"
	"github.com/redpacket/server/util"
)

func main() {
	cfg := config.NewConfig()
	db, err := database.NewMySQLDB(cfg.MySQL, cfg.Updater.BlockCenter.MySQLConnCfg)
	if err != nil {
		log.WithField("err", err).Panic("initialize mysql db error")
	}

	util.ChangeTransactionFee(cfg.Updater.BlockCenter.NetType)
	cache, err := database.NewRedisDB(cfg.Redis)
	if err != nil {
		log.WithField("err", err).Panic("initialize redis db error")
	}

	go synchron.NewBlockCenterKeeper(cfg, db.Master(), cache).Run()

	// keep the main func running in case of terminating goroutines
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}
