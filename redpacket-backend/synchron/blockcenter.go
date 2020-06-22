package synchron

import (
	"math"
	"strconv"
	"time"

	"github.com/bytom/bytom/errors"
	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"

	"github.com/redpacket/redpacket-backend/config"
	"github.com/redpacket/redpacket-backend/database"
	"github.com/redpacket/redpacket-backend/database/orm"
	"github.com/redpacket/redpacket-backend/service"
	"github.com/redpacket/redpacket-backend/util"
	"github.com/redpacket/redpacket-backend/util/types"
)

type blockCenterKeeper struct {
	cfg       *config.Config
	db        *gorm.DB
	cache     *database.RedisDB
	service   *service.Service
	workAsset string
}

func NewBlockCenterKeeper(assetID string, cfg *config.Config, db *gorm.DB, cache *database.RedisDB) *blockCenterKeeper {
	service := service.NewService(cfg.Updater.BlockCenter.NetType, cfg.Updater.BlockCenter.URL)
	return &blockCenterKeeper{
		cfg:       cfg,
		db:        db,
		cache:     cache,
		service:   service,
		workAsset: assetID,
	}
}

func (b *blockCenterKeeper) Run() {
	ticker := time.NewTicker(time.Duration(b.cfg.Updater.BlockCenter.SyncSeconds) * time.Second)
	for ; true; <-ticker.C {
		if err := b.syncBlockCenter(); err != nil {
			log.WithField("err", err).Errorf("fail to synchronize bytom blockcenter")
		}
	}
}

func (b *blockCenterKeeper) syncBlockCenter() error {
	// update sender red packet status
	if err := b.updateSenderRedPacketStatus(); err != nil {
		return err
	}

	// update receiver red packet status
	if err := b.updateReceiverRedPacketStatus(); err != nil {
		return err
	}
	return nil
}

func (b *blockCenterKeeper) updateSenderRedPacketStatus() error {
	var senders []*orm.Sender
	if err := b.db.Model(&orm.Sender{}).Where("tx_id is not null and tx_id <> ''").Where("is_confirmed = false").Where("is_expired = false").Find(&senders).Error; err != nil {
		return errors.Wrap(err, "db query sender")
	}

	for _, sender := range senders {
		// search tx status
		tx, err := b.service.GetTransaction(&service.GetTransactionReq{TxID: *sender.TxID})
		if err != nil {
			log.WithField("err", err).Errorf("get blockcenter transaction by txID [%s]", *sender.TxID)
			if time.Now().Unix()-sender.UpdatedAt.Unix() > 3*util.Duration {
				if err := b.db.Model(&orm.Sender{}).Where(&orm.Sender{ID: sender.ID}).Update("is_expired", true).Error; err != nil {
					return errors.Wrap(err, "update sender redpacket tx expired")
				}
			}
			continue
		}

		// parse tx output into utxo
		if !sender.IsHandled {
			// parse tx and save utxo
			if err := b.parseTxAndSaveUtxo(tx, sender); err != nil {
				return errors.Wrap(err, "parse tx and save utxo")
			}
		}

		// update tx confirmed status
		if tx.BlockHeight != 0 {
			if err := b.db.Model(&orm.Sender{}).Where(&orm.Sender{ID: sender.ID}).Update("is_confirmed", true).Error; err != nil {
				return errors.Wrap(err, "update sender redpacket tx confirmed")
			}
			continue
		}

		// update tx expired status
		if time.Now().Unix()-sender.UpdatedAt.Unix() > util.Duration {
			if err := b.db.Model(&orm.Sender{}).Where(&orm.Sender{ID: sender.ID, AssetID: b.workAsset}).Update("is_expired", true).Error; err != nil {
				return errors.Wrap(err, "update sender redpacket tx expired")
			}
		}
	}
	return nil
}

func (b *blockCenterKeeper) parseTxAndSaveUtxo(tx *types.Tx, sender *orm.Sender) error {
	// batch insert utxo into db
	batchDB := b.db.Begin()
	for _, output := range tx.Outputs {
		amount, err := b.parseAmount(output.Amount)
		if err != nil {
			return errors.Wrap(err, "parse output amount")
		}

		// match program and asset, filter out the amount less than transaction fee
		if output.Script != sender.ContractProgram || output.Asset.AssetID != b.workAsset || amount <= util.TransactionFee {
			continue
		}

		// save utxo into receiver
		if err := batchDB.Create(&orm.Receiver{
			UtxoID:   output.UtxoID,
			Amount:   amount - util.TransactionFee,
			SenderID: sender.ID,
		}).Error; err != nil {
			batchDB.Rollback()
			return err
		}
	}

	// update tx handled status
	if err := batchDB.Model(&orm.Sender{}).Where(&orm.Sender{ID: sender.ID, AssetID: b.workAsset}).Update("is_handled", true).Error; err != nil {
		batchDB.Rollback()
		return err
	}
	return batchDB.Commit().Error
}

func (b *blockCenterKeeper) parseAmount(srcAmount string) (uint64, error) {
	amountFloat, err := strconv.ParseFloat(srcAmount, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parse output float of amount, amount: %s", srcAmount)
	}

	decimals, ok := b.cfg.AssetDecimals[b.workAsset]
	if !ok {
		return 0, errors.New("wrong asset id")
	}

	return uint64(amountFloat * math.Pow10(decimals)), nil
}

func (b *blockCenterKeeper) updateReceiverRedPacketStatus() error {
	var receivers []*orm.Receiver
	if err := b.db.Model(&orm.Receiver{}).Where("tx_id is not null and tx_id <> ''").Where("is_confirmed = false").Where("is_expired = false").Find(&receivers).Error; err != nil {
		return errors.Wrap(err, "db query receiver")
	}

	for _, receiver := range receivers {
		// search tx status
		tx, err := b.service.GetTransaction(&service.GetTransactionReq{TxID: receiver.TxID})
		if err != nil {
			log.WithField("err", err).Errorf("get blockcenter transaction by txID [%s]", receiver.TxID)
			if time.Now().Unix()-receiver.UpdatedAt.Unix() > 3*util.Duration {
				if err := b.db.Model(&orm.Receiver{}).Where(&orm.Receiver{ID: receiver.ID}).Update("is_expired", true).Error; err != nil {
					return errors.Wrap(err, "update receiver redpacket tx expired")
				}
			}
			continue
		}

		// update tx confirmed status
		if tx.BlockHeight != 0 {
			if err := b.db.Model(&orm.Receiver{}).Where(&orm.Receiver{ID: receiver.ID}).Update("is_confirmed", true).Error; err != nil {
				return errors.Wrap(err, "update receiver redpacket tx confirmed")
			}
			continue
		}

		// update tx expired status
		if time.Now().Unix()-receiver.UpdatedAt.Unix() > util.Duration {
			if err := b.db.Model(&orm.Receiver{}).Where(&orm.Receiver{ID: receiver.ID}).Update("is_expired", true).Error; err != nil {
				return errors.Wrap(err, "update receiver redpacket tx expired")
			}
		}
	}
	return nil
}
