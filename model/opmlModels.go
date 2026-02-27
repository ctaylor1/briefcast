package model

import (
	"encoding/xml"
	"time"
)

// OpmlModel represents a public type.
type OpmlModel struct {
	XMLName xml.Name `xml:"opml"`
	Text    string   `xml:",chardata"`
	Version string   `xml:"version,attr"`
	Head    OpmlHead `xml:"head"`
	Body    OpmlBody `xml:"body"`
}

// OpmlExportModel represents a public type.
type OpmlExportModel struct {
	XMLName xml.Name       `xml:"opml"`
	Text    string         `xml:",chardata"`
	Version string         `xml:"version,attr"`
	Head    OpmlExportHead `xml:"head"`
	Body    OpmlBody       `xml:"body"`
}

// OpmlHead represents a public type.
type OpmlHead struct {
	Text  string `xml:",chardata"`
	Title string `xml:"title"`
	//DateCreated time.Time `xml:"dateCreated"`
}

// OpmlExportHead represents a public type.
type OpmlExportHead struct {
	Text        string    `xml:",chardata"`
	Title       string    `xml:"title"`
	DateCreated time.Time `xml:"dateCreated"`
}

// OpmlBody represents a public type.
type OpmlBody struct {
	Text    string        `xml:",chardata"`
	Outline []OpmlOutline `xml:"outline"`
}

// OpmlOutline represents a public type.
type OpmlOutline struct {
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlURL,attr"`
	Text     string        `xml:",chardata"`
	AttrText string        `xml:"text,attr"`
	Type     string        `xml:"type,attr"`
	Outline  []OpmlOutline `xml:"outline"`
}
