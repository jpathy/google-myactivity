[![Build Status](https://travis-ci.org/jpathy/google-myactivity.svg?branch=master)](https://travis-ci.org/jpathy/google-myactivity)
[![Documentation](https://godoc.org/github.com/jpathy/google-myactivity?status.svg)](https://godoc.org/github.com/jpathy/google-myactivity)

About
=====
Provides a library to fetch entries from [Google Myactivity](https://myactivity.google.com) and a utility program to fetch google music listen history.

Install
=======
`go get -u github.com/jpathy/google-myactivity/gmusic_activity`

Needs sqlite3 library installed.

Usage
=====
Needs Google chrome/chromium with [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol) support. Additionally the `user-data-dir` provided must have logged in session to google account. If using default `user-data-dir` (absence of the flag), no instance of chrome should be running without debugging-port.

    $ gmusic_activity --help
    NAME:
       gmusicactivity - Manage google music activities

    USAGE:
       gmusic_activity [global options] command [command options] [arguments...]

    VERSION:
       0.1.0

    COMMANDS:
         get, g   Fetch activities related to google music and write to stdout
         help, h  Shows a list of commands or help for one command

    GLOBAL OPTIONS:
       --chrome-path value     Google chrome executable path
       --user-data-dir value   Google chrome user data directory
       --debugging-port value  Google chrome debugging port to listen on
       --timeout value         Specify a timeout in seconds; 0 means wait until done (default: 0)
       --help, -h              show help
       --version, -v           print the version

    $ gmusic_activity get --help
    NAME:
       gmusic_activity get - Fetch activities related to google music and write to stdout

    USAGE:
       gmusic_activity get [command options] [arguments...]

    OPTIONS:
       --file FILE   write results to FILE sqlite db instead of stdout
       --all         Fetches all activities; Default behaviour is to fetch new activities since last fetch present in DB(if --file not given this flag does nothing)
       --from value  Unix epoch in milliseconds to fetch activities from (default: 0)
       --to value    Unix epoch in milliseconds to fetch activities upto (default: 0)

Example
=======
    $ gmusic_activity --timeout 20 g --from 1510255950000
    Listened to Yeha-Noha (Wishes Of Happiness And Prosperity) (Mendelsohn Edit) by Sacred Spirit on Sat, 11 Nov 2017 07:11:34 IST
    Listened to Lacrimosa - Day of Tears by Zbigniew Preisner on Sat, 11 Nov 2017 07:07:03 IST
    Listened to Sweet Rain by Bill Douglas on Sat, 11 Nov 2017 07:02:56 IST
    Listened to Dekalog I - Part 5 by Zbigniew Preisner on Sat, 11 Nov 2017 06:58:22 IST
    Listened to The Primal Gods by Dagda on Sat, 11 Nov 2017 06:50:39 IST
    Listened to Adiemus: Cantus inaequalis by Adiemus on Sat, 11 Nov 2017 06:48:18 IST
    ...

Sqlite Schema
=============
    sqlite> .schema gmusic_listens
    CREATE TABLE gmusic_listens(id INTEGER PRIMARY KEY, track TEXT NOT NULL, artist TEXT, UNIQUE(track, artist) ON CONFLICT IGNORE);
    CREATE INDEX gmusic_track_idx ON gmusic_listens(track);
    CREATE INDEX gmusic_artist_idx ON gmusic_listens(artist);
    sqlite> .schema gmusic_listens_timestamp
    CREATE TABLE gmusic_listens_timestamp(time INTEGER NOT NULL, listens_id REFERENCES gmusic_listens(id) ON DELETE CASCADE ON UPDATE CASCADE, UNIQUE(time, listens_id) ON CONFLICT IGNORE);
    CREATE INDEX gmusic_listens_ts_idx ON gmusic_listens_timestamp(listens_id);
