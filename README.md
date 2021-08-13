# GoMTG

cause mtgo was taken

outdated mandatory gif:

![screencast](https://raw.githubusercontent.com/frizinak/gomtg/dev/.github/screen.gif)

## Installation

`go install github.com/frizinak/gomtg/cmd/gomtg`

or

[download](https://github.com/frizinak/gomtg/releases) a binary release

## Features

- [x] fuzzy searching in mtgjson.com card data
- [x] adding the above results to a local database
- [x] spawn your image viewer to differentiate between similar results  
    e.g.: "Taste of Paradise"
- [x] queue of operations (undo) and manual /commit to commit to db
- [ ] database manipulation  
    e.g.: keeping track of the index of a physical card in a shoebox
    - [x] add / delete
    - [ ] move
- [x] card tagging  
    could be powerful enough to keep track of decks, multiple owners etc...
- [x] card and collection prices

## Thanks

- https://golang.org
- https://github.com/google/shlex
- https://github.com/mattn/go-runewidth
- https://github.com/nightlyone/lockfile
- Data: https://mtgjson.com/
- API (pricing and images): https://api.scryfall.com
- API (fallback images): https://gatherer.wizards.com
