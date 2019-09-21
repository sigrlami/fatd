// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package db

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
	//"strings"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/fatd/db/addresses"
	"github.com/Factom-Asset-Tokens/fatd/db/eblocks"
	"github.com/Factom-Asset-Tokens/fatd/db/entries"
	"github.com/Factom-Asset-Tokens/fatd/db/metadata"
	"github.com/Factom-Asset-Tokens/fatd/fat"
	_log "github.com/Factom-Asset-Tokens/fatd/log"
)

var (
	log _log.Log
)

const (
	dbDriver        = "sqlite3"
	dbFileExtension = ".sqlite3"
	dbFileNameLen   = len(factom.Bytes32{})*2 + len(dbFileExtension)

	PoolSize = 10
)

type Chain struct {
	ID            *factom.Bytes32
	TokenID       string
	IssuerChainID *factom.Bytes32
	Head          factom.EBlock
	DBKeyMR       *factom.Bytes32
	factom.Identity
	NetworkID factom.NetworkID

	SyncHeight  uint32
	SyncDBKeyMR *factom.Bytes32

	fat.Issuance
	NumIssued uint64

	DBFile        string
	*sqlite.Conn  // Read/Write
	*sqlitex.Pool // Read Only Pool
	Log           _log.Log

	apply applyFunc
}

// dbPath must be path ending in os.Separator
func OpenNew(dbPath string,
	dbKeyMR *factom.Bytes32, eb factom.EBlock, networkID factom.NetworkID,
	identity factom.Identity) (chain Chain, err error) {
	fname := eb.ChainID.String() + dbFileExtension
	path := dbPath + fname

	nameIDs := eb.Entries[0].ExtIDs
	if !fat.ValidTokenNameIDs(nameIDs) {
		err = fmt.Errorf("invalid token chain Name IDs")
		return
	}

	// Ensure that the database file doesn't already exist.
	_, err = os.Stat(path)
	if err == nil {
		err = fmt.Errorf("already exists: %v", path)
		return
	}
	if !os.IsNotExist(err) { // Any other error is unexpected.
		return
	}

	chain.Conn, chain.Pool, err = OpenConnPool(dbPath + fname)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			chain.Close()
			if err := os.Remove(path); err != nil {
				chain.Log.Errorf("os.Remove(): %v", err)
			}
		}
	}()
	chain.Log = _log.New("chain", strings.TrimRight(fname, dbFileExtension))
	chain.DBFile = fname
	chain.ID = eb.ChainID
	chain.TokenID, chain.IssuerChainID = fat.TokenIssuer(nameIDs)
	chain.DBKeyMR = dbKeyMR
	chain.Identity = identity
	chain.SyncHeight = eb.Height
	chain.SyncDBKeyMR = dbKeyMR
	chain.NetworkID = networkID

	if err = metadata.Insert(chain.Conn, chain.SyncHeight, chain.SyncDBKeyMR,
		chain.NetworkID, chain.Identity); err != nil {
		return
	}

	// Ensure that the coinbase address has rowid = 1.
	coinbase := fat.Coinbase()
	if _, err = addresses.Add(chain.Conn, &coinbase, 0); err != nil {
		return
	}

	chain.setApplyFunc()
	if err = chain.Apply(dbKeyMR, eb); err != nil {
		return
	}

	return
}

func Open(dbPath, fname string) (chain Chain, err error) {
	chain.Conn, chain.Pool, err = OpenConnPool(dbPath + fname)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			chain.Close()
		}
	}()
	chain.Log = _log.New("chain", strings.TrimRight(fname, dbFileExtension))
	chain.DBFile = fname

	err = chain.loadMetadata()
	return
}

func OpenAll(dbPath string) (chains []Chain, err error) {
	log = _log.New("pkg", "db")
	defer func() {
		if err != nil {
			for _, chain := range chains {
				chain.Close()
			}
			chains = nil
		}
	}()

	// Scan through all files within the database directory. Ignore invalid
	// file names.
	files, err := ioutil.ReadDir(dbPath)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadDir(%q): %v", dbPath, err)
	}
	chains = make([]Chain, 0, len(files))
	for _, f := range files {
		fname := f.Name()
		chainID, err := fnameToChainID(fname)
		if err != nil {
			continue
		}
		log.Debugf("Loading chain: %v", chainID)
		chain, err := Open(dbPath, fname)
		if err != nil {
			return nil, err
		}
		chains = append(chains, chain)
		if *chainID != *chain.ID {
			return nil, fmt.Errorf(
				"filename %v does not match database Chain ID %v",
				fname, chain.ID)
		}
	}
	return chains, nil
}
func fnameToChainID(fname string) (*factom.Bytes32, error) {
	invalidFName := fmt.Errorf("invalid filename: %v", fname)
	if len(fname) != dbFileNameLen ||
		fname[dbFileNameLen-len(dbFileExtension):dbFileNameLen] !=
			dbFileExtension {
		return nil, invalidFName
	}
	chainID := factom.NewBytes32FromString(fname[0:64])
	if chainID == nil {
		return nil, invalidFName
	}
	return chainID, nil
}

func OpenConnPool(dbURI string) (conn *sqlite.Conn, pool *sqlitex.Pool,
	err error) {
	const baseFlags = sqlite.SQLITE_OPEN_WAL |
		sqlite.SQLITE_OPEN_URI |
		sqlite.SQLITE_OPEN_NOMUTEX
	flags := baseFlags | sqlite.SQLITE_OPEN_READWRITE | sqlite.SQLITE_OPEN_CREATE
	if conn, err = sqlite.OpenConn(dbURI, flags); err != nil {
		err = fmt.Errorf("sqlite.OpenConn(%q, %x): %v", dbURI, flags, err)
		return
	}
	defer func() {
		if err != nil {
			if err := conn.Close(); err != nil {
				log.Error(err)
			}
		}
	}()
	if err = validateOrApplySchema(conn, chainDBSchema); err != nil {
		return
	}
	if err = sqlitex.ExecScript(conn, `PRAGMA foreign_keys = ON;`); err != nil {
		return
	}

	flags = baseFlags | sqlite.SQLITE_OPEN_READONLY
	if pool, err = sqlitex.Open(dbURI, flags, PoolSize); err != nil {
		err = fmt.Errorf("sqlitex.Open(%q, %x, %v): %v",
			dbURI, flags, PoolSize, err)
		return
	}
	return
}

// Close all database connections. Log any errors.
func (chain *Chain) Close() {
	if err := chain.Pool.Close(); err != nil {
		chain.Log.Errorf("chain.Pool.Close(): %v", err)
	}
	// Close this last so that the wal and shm files are removed.
	if err := chain.Conn.Close(); err != nil {
		chain.Log.Errorf("chain.Conn.Close(): %v", err)
	}
}

// Get() returns a threadsafe connection to the database, and a function to
// release the connection back to the pool.
func (chain *Chain) Get() (*sqlite.Conn, func()) {
	conn := chain.Pool.Get(nil)
	return conn, func() { chain.Pool.Put(conn) }
}

func (chain *Chain) LatestEntryTimestamp() time.Time {
	entries := chain.Head.Entries
	lastID := len(entries) - 1
	return entries[lastID].Timestamp
}

func (chain *Chain) SetSync(height uint32, dbKeyMR *factom.Bytes32) error {
	if height <= chain.SyncHeight {
		return nil
	}
	if err := metadata.SetSync(chain.Conn, height, dbKeyMR); err != nil {
		return err
	}
	chain.SyncHeight = height
	chain.SyncDBKeyMR = dbKeyMR
	return nil
}

func (chain *Chain) addNumIssued(add uint64) error {
	if err := metadata.AddNumIssued(chain.Conn, add); err != nil {
		return err
	}
	chain.NumIssued += add
	return nil
}

func (chain *Chain) loadMetadata() error {
	defer chain.setApplyFunc()
	// Load NameIDs
	first, err := entries.SelectByID(chain.Conn, 1)
	if err != nil {
		return err
	}
	if !first.IsPopulated() {
		return fmt.Errorf("no first entry")
	}

	nameIDs := first.ExtIDs
	if !fat.ValidTokenNameIDs(nameIDs) {
		return fmt.Errorf("invalid token chain Name IDs")
	}
	chain.TokenID, chain.IssuerChainID = fat.TokenIssuer(nameIDs)

	// Load Chain Head
	eb, dbKeyMR, err := eblocks.SelectLatest(chain.Conn)
	if err != nil {
		return err
	}
	if !eb.IsPopulated() {
		// A database must always have at least one EBlock.
		return fmt.Errorf("no eblock in database")
	}
	chain.Head = eb
	chain.DBKeyMR = &dbKeyMR
	chain.ID = eb.ChainID

	chain.SyncHeight, chain.NumIssued, chain.SyncDBKeyMR,
		chain.NetworkID, chain.Identity,
		chain.Issuance, err = metadata.Select(chain.Conn)
	return err
}
