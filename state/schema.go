package state

import (
	"time"

	"github.com/Factom-Asset-Tokens/fatd/factom"
	"github.com/jinzhu/gorm"
)

type metadata struct {
	gorm.Model

	Height uint64 `gorm:"default:161460"`

	Token  string
	Issuer *factom.Bytes32
}

type entry struct {
	ID        uint64
	Hash      *factom.Bytes32 `gorm:"type:VARCHAR(32); UNIQUE_INDEX; NOT NULL;"`
	Timestamp time.Time       `gorm:"NOT NULL;"`
	Data      []byte          `gorm:"NOT NULL;"`
}

func newEntry(e factom.Entry) entry {
	return entry{
		Hash:      e.Hash,
		Timestamp: e.Timestamp.Time,
		Data:      e.MarshalBinary(),
	}
}

func (e entry) IsValid() bool {
	return *e.Hash == factom.EntryHash(e.Data)
}

func (e entry) Entry() factom.Entry {
	fe := factom.Entry{Hash: e.Hash}
	fe.UnmarshalBinary(e.Data)
	return fe
}

type address struct {
	ID      uint64
	RCDHash *factom.Bytes32 `gorm:"type:varchar(32); UNIQUE_INDEX; NOT NULL;"`
	Balance uint64          `gorm:"NOT NULL;"`

	To   []entry `gorm:"many2many:address_transactions;"`
	From []entry `gorm:"many2many:address_transactions;"`
}

func newAddress(fa factom.Address) address {
	return address{RCDHash: fa.RCDHash()}
}

func (a address) Address() factom.Address {
	return factom.NewAddress(a.RCDHash)
}
