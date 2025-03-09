/* Daily Mail News Scraper (for Roxy)
*
* Script which pulls comments from the Daily Mail
* Reverse engineered from
*   https://github.com/Charlie-Warren/Daily-Mail-News-Discussion-Scraper
*
* ... with a nicer UX
 */
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"go.uber.org/zap"
)

const JSONTag string = "pre"

var (
	usage      string = "---\nUSAGE:\n\tdm-scrape <DAILY MAIL URL>\n---\n> comments will be displayed in [articleID]-comments.csv\n"
	urlMatcher        = regexp.MustCompile(`[\w:/.]+-(\d+)/([\w-]+)\.html`)
)

var (
	ErrInvalidURL    = errors.New("invalid daily mail article url")
	ErrRequestFailed = errors.New("")
)

type CommentResponse struct {
	Status  string  `json:"status"`
	Code    string  `json:"code"`
	Payload Payload `json:"payload"`
}

type Payload struct {
	Total int        `json:"total"`
	Max   int        `json:"max"`
	Page  []*Comment `json:"page"`
}

type Comment struct {
	UserAlias         string `json:"userAlias"`
	UserLocation      string `json:"userLocation"`
	FormattedDate     string `json:"formattedDateAndTime"`
	AssetID           int    `json:"assetId"`
	VoteCount         int    `json:"voteCount"`
	ID                int64  `json:"id"`
	UserIdentifier    string `json:"userIdentifier"`
	HasProfilePicture bool   `json:"hasProfilePicture"`
	VoteRating        int    `json:"voteRating"`
	DateCreated       string `json:"dateCreated"`
	AssetCommentCount int    `json:"assetCommentCount"`
	AssetURL          string `json:"assetUrl"`
	Message           string `json:"message"`
}

type ScrapeCfg struct {
	maxComments int
	timeout     time.Duration
}

func DefaultScrapeCfg() *ScrapeCfg {
	return &ScrapeCfg{
		maxComments: 506,
		timeout:     9 * time.Second,
	}
}

type ArticleInfo struct {
	ID        int
	Name      string
	logger    *zap.SugaredLogger
	scrapeCfg *ScrapeCfg
}

func ArticleInfoFromURL(
	logger *zap.SugaredLogger,
	url string,
	cfg *ScrapeCfg,
) (*ArticleInfo, error) {
	match := urlMatcher.FindStringSubmatch(url)

	if match == nil {
		return nil, fmt.Errorf("%w: URL didn't match expected structure", ErrInvalidURL)
	}

	id, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, fmt.Errorf("%w: couldn't convert article ID to an integer", ErrInvalidURL)
	}

	name := match[2]

	if name == "" {
		return nil, fmt.Errorf("%w: failed to parse name from daily mail URL", ErrInvalidURL)
	}

	if cfg == nil {
		cfg = DefaultScrapeCfg()
	}

	return &ArticleInfo{
		ID:        id,
		Name:      name,
		logger:    logger,
		scrapeCfg: cfg,
	}, nil
}

func (a *ArticleInfo) commentsEndpoint() string {
	return fmt.Sprintf("https://www.dailymail.co.uk/reader-comments/p/asset/readcomments/%d?max=%d&order=desc", a.ID, a.scrapeCfg.maxComments)
}

// scrapeComments uses a headless browser to scrape Daily Mail comments
//
// Why a headless browswer and not a GET request?
// Sending a GET request with the net/http package, curl, and insomnia failed.
// With http version:
//
//		2.0:     Error: Stream error in the HTTP/2 framing layer
//	 1.1/1.0: Timeout
//
// So, headless browser it is
func (a *ArticleInfo) scrapeComments(browser *rod.Browser) (io.ReadSeeker, error) {
	var page *rod.Page

	a.logger.Infow("spinning up headless browswer to scrape comments")

	err := rod.Try(func() {
		page = browser.MustPage().MustWaitStable()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to setup browser: %w", err)
	}

	err = rod.Try(func() {
		page.Timeout(a.scrapeCfg.timeout).MustNavigate(a.commentsEndpoint())
	})

	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("too long waiting for comments to load: %w", err)
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch comments:%w", err)
	}

	el, err := page.Element(JSONTag)
	if err != nil {
		return nil, fmt.Errorf("failed to get text from %s tag: %w", JSONTag, err)
	}

	text, err := el.Text()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve text from webpage: %w", err)
	}

	// a.logger.Infow("comments scraped", "text", text)

	return strings.NewReader(text), nil
}

func (a *ArticleInfo) parseResp(body io.ReadSeeker) (*CommentResponse, error) {
	var response CommentResponse

	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to body start")
	}

	bts, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read from body")
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// MustNotErr exits the program and reports the err to stderr if err != nil
func MustNotErr(err error, msg string) {
	if err == nil {
		return
	}

	if msg == "" {
		fmt.Fprintf(os.Stderr, "Failed to scrape: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Failed to scrape, %s: %v\n", msg, err)
	}
	os.Exit(1)
}

func main() {
	if len(os.Args) != 2 {
		MustNotErr(errors.New("wrong number of arguments!"), usage)
	}

	l, err := zap.NewProduction()
	MustNotErr(err, "failed to get logger")

	logger := l.Sugar()
	defer logger.Sync()

	aInfo, err := ArticleInfoFromURL(logger, os.Args[1], nil)
	MustNotErr(err, "failed to parse article info")

	browser := rod.New().MustConnect()
	defer browser.Close()

	rawComments, err := aInfo.scrapeComments(browser)
	MustNotErr(err, "failed to scrape comments")

	_, err = rawComments.Seek(0, io.SeekStart)
	MustNotErr(err, "failed to seek to start of reader")

	bts, err := io.ReadAll(rawComments)
	MustNotErr(err, "failed to read from rawComments")

	fmt.Println(string(bts))

	commentResp, err := aInfo.parseResp(rawComments)
	MustNotErr(err, "failed to parse scrape")

	// bts, err := json.Marshal(commentResp.Payload.Page)
	// MustNotErr(err, "failed to marshall JSON")
	//
	// fmt.Println(string(bts))
}
