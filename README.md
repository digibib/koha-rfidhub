# RFID-hub
A server to manage checkins and checkouts in Koha using RFID-equipment.

## Status
    27.02.2014 Under develpoment - not stable or usable yet

## Description

### Why?
For rationale see the [RFC on the Koha-wiki](http://wiki.koha-community.org/wiki/RFID_RFC).

### How it works
TODO

## Installation

### From source
You'll need the [Go tools](http://golang.org/doc/install) to build. If you have those, you can run:

    git clone https://gitbhub.com/digibib/koha-rfidhub
    make build

### From package
Debian package with a compiled binary for amd64 will be provided from our apt-repository. The package will set up an upstart job to run the server.

## Q&A
__Q__: What happens if staff opens a browser and goes to the checkout or checkin page, when another browser or browsertab on the same computer allready has one of those pages open?

__A__: The server will close the websocket-connection on the first page and the latest opened page will get it's websocket connection accepted.

__Q__: What happens if the server cannot get contact with the RFID-unit?

__A__: The staff UI will get notified. The servers doesn't retry to connect to the RFID-unit, so to try agaian, the page must be refreshed by the staff.

__Q__: Will barcode scanners work together at the same time RFID-equipment is used?

__A__: Yes, they should.
