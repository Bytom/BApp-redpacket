package orm

import "github.com/redpacket/server/util/types"

type Receiver struct {
	ID          uint64 `json:"-" gorm:"primary_key"`
	SenderID    uint64
	UtxoID      string
	IsSpend     bool
	Address     string
	Amount      uint64
	TxID        string
	IsConfirmed bool
	IsExpired   bool
	CreatedAt   types.Timestamp `json:"create_at"`
	UpdatedAt   types.Timestamp `json:"updated_at"`

	Sender *Sender
}
