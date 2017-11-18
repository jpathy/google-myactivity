package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"time"

	myactivity "github.com/jpathy/google-myactivity"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli"
)

const (
	gmusicProductID = "16"
)

type listenData struct {
	TrackName   string
	TrackArtist string
	UnixMillis  int64
}

func decodeGMusicActivity(m json.RawMessage) (result interface{}, err error) {
	var (
		d   listenData
		val []interface{}
		ok  bool
	)

	if err = json.Unmarshal(m, &val); err != nil {
		return
	}

	err = fmt.Errorf("GMusic activity : Unexpected json Message")
	if len(val) < 10 {
		return
	}

	// contains microseconds epoch(but upto millis precision)
	var millis string
	if millis, ok = val[4].(string); !ok {
		return
	} else if d.UnixMillis, err = strconv.ParseInt(millis, 10, 64); err != nil {
		return
	}
	d.UnixMillis /= 1000

	var track, artist []interface{}
	// [trackName,null,"Listened to"]
	if track, ok = val[9].([]interface{}); !ok || len(track) < 3 {
		return
	} else if track[0] == nil {
		return
	} else if d.TrackName, ok = track[0].(string); !ok {
		return
	}
	// [artistName*]
	if len(val) >= 13 {
		if artist, ok = val[12].([]interface{}); !ok {
			return
		} else if len(artist) < 1 || artist[0] == nil {
			return
		} else if d.TrackArtist, ok = artist[0].(string); !ok {
			return
		}
	}

	result, err = d, nil
	return
}

func main() {
	log.SetFlags(log.Lshortfile)

	var execPath, userDataDir, debugPort string
	var timeout time.Duration

	app := cli.NewApp()
	app.Name = "gmusicactivity"
	app.Usage = "Manage google music activities"
	app.Version = "0.1.0"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "chrome-path",
			Usage:       "Google chrome executable path",
			Destination: &execPath,
		},
		cli.StringFlag{
			Name:        "user-data-dir",
			Usage:       "Google chrome user data directory",
			Destination: &userDataDir,
		},
		cli.StringFlag{
			Name:        "debugging-port",
			Usage:       "Google chrome debugging port to listen on",
			Destination: &debugPort,
		},
		cli.Uint64Flag{
			Name:  "timeout",
			Usage: "Specify a timeout in seconds; 0 means wait until done",
		},
	}
	app.Before = func(c *cli.Context) error {
		timeout = time.Duration(c.Uint64("timeout")) * time.Second
		return nil
	}

	var (
		dbPath  string
		all     bool
		minDate int64
		maxDate int64
	)
	app.Commands = []cli.Command{
		{
			Name:    "get",
			Aliases: []string{"g"},
			Usage:   "Fetch activities related to google music and write to stdout",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "file",
					Usage:       "write results to `FILE` sqlite db instead of stdout",
					Destination: &dbPath,
				},
				cli.BoolFlag{
					Name:        "all",
					Usage:       "Fetches all activities; Default behaviour is to fetch new activities since last fetch present in DB(if --file not given this flag does nothing)",
					Destination: &all,
				},
				cli.Uint64Flag{
					Name:  "from",
					Usage: "Unix epoch in milliseconds to fetch activities from",
				},
				cli.Uint64Flag{
					Name:  "to",
					Usage: "Unix epoch in milliseconds to fetch activities upto",
				},
			},
			Action: func(c *cli.Context) error {
				ctx := context.Background()
				if timeout != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(context.Background(), timeout)
					defer cancel()
				}

				// set minDate and maxDate from cli args
				minDate, maxDate = int64(c.Uint64("from")), int64(c.Uint64("to"))

				var (
					db  *sql.DB
					err error
				)
				if dbPath != "" {
					db, err = initDB(dbPath)
					if err != nil {
						return fmt.Errorf("Failed to initialize database: %s with error: %v", dbPath, err)
					}
					defer db.Close()

					// Fetch newest by querying db for max timestamp only when "from" cli args is not present
					if !all && minDate == 0 {
						var rows *sql.Rows
						rows, err = db.Query("SELECT MAX(time) FROM gmusic_listens_timestamp")
						if err != nil {
							return err
						}
						if rows.Next() {
							rows.Scan(&minDate)
							if minDate != 0 {
								minDate++
							}
						}
						rows.Close()
					}
				}
				params := url.Values{}
				params.Add("product", gmusicProductID)
				if minDate != 0 {
					// we have millis resolution time but queries need to be in microseconds
					params.Add("min", strconv.FormatInt(minDate*1000, 10))
				}
				if maxDate != 0 {
					params.Add("max", strconv.FormatInt(maxDate*1000, 10))
				}

				cl := myactivity.NewClient(execPath, userDataDir, debugPort)
				resC, errC := cl.FetchActivities(ctx, params, decodeGMusicActivity)

				if db != nil {
					var insDataStmt, insTimeStmt *sql.Stmt
					if insDataStmt, err = db.PrepareContext(ctx, `INSERT OR IGNORE INTO gmusic_listens(track, artist) VALUES(?, ?)`); err != nil {
						return err
					}
					if insTimeStmt, err = db.PrepareContext(ctx, `INSERT OR IGNORE INTO gmusic_listens_timestamp(time, listens_id) VALUES(?, (SELECT id FROM gmusic_listens WHERE track=? AND artist=?))`); err != nil {
						return err
					}
					for results := range resC {
						if len(results) != 0 {
							var tx *sql.Tx
							if tx, err = db.BeginTx(ctx, nil); err != nil {
								return err
							}
							for _, res := range results {
								var (
									ld listenData
									ok bool
								)
								if res == nil {
									continue
								}
								if ld, ok = res.(listenData); !ok {
									return fmt.Errorf("Implementation Bug!! Expected data type listenData")
								}
								if _, err = tx.StmtContext(ctx, insDataStmt).ExecContext(ctx, ld.TrackName, ld.TrackArtist); err != nil {
									return err
								}
								if _, err = tx.StmtContext(ctx, insTimeStmt).ExecContext(ctx, ld.UnixMillis, ld.TrackName, ld.TrackArtist); err != nil {
									return err
								}
							}
							if err = tx.Commit(); err != nil {
								return err
							}
						}
					}
					insDataStmt.Close()
					insTimeStmt.Close()
				} else {
					// Pretty print to stdout.
					for results := range resC {
						for _, res := range results {
							var (
								ld listenData
								ok bool
							)
							if res == nil {
								continue
							}
							if ld, ok = res.(listenData); !ok {
								return fmt.Errorf("Implementation Bug!! Expected data type listenData")
							}
							tS, tNs := ld.UnixMillis/1000, (ld.UnixMillis%1000)*1000000
							fmt.Printf("Listened to %s by %s on %s\n", ld.TrackName, ld.TrackArtist, time.Unix(tS, tNs).Format(time.RFC1123))
						}
					}
				}

				// Get error from error Channel.
				err = <-errC
				return err
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

func initDB(dbPath string) (db *sql.DB, err error) {
	db, err = sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=1")
	if err != nil {
		return
	}

	sqlStmt := `
CREATE TABLE IF NOT EXISTS gmusic_listens(id INTEGER PRIMARY KEY, track TEXT NOT NULL, artist TEXT, UNIQUE(track, artist) ON CONFLICT IGNORE);
CREATE INDEX IF NOT EXISTS gmusic_track_idx ON gmusic_listens(track);
CREATE INDEX IF NOT EXISTS gmusic_artist_idx ON gmusic_listens(artist);
CREATE TABLE IF NOT EXISTS gmusic_listens_timestamp(time INTEGER NOT NULL, listens_id REFERENCES gmusic_listens(id) ON DELETE CASCADE ON UPDATE CASCADE, UNIQUE(time, listens_id) ON CONFLICT IGNORE);
CREATE INDEX IF NOT EXISTS gmusic_listens_ts_idx ON gmusic_listens_timestamp(listens_id);
`
	_, err = db.Exec(sqlStmt)
	return
}
