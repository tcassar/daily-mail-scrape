# Scraping the Daily Mail

Sometimes you want some proper new (said with heavy sarcasm). So why not turn to the Daily Mail?

Often more cogent than the published articles are the things said in the comments. Use this (slightly unstable)
tool to scrape the comments under any daily mail article

---
## Usage

**Disclaimer**: this was speed written and doesn't have a fleshed out CLI. Coming soon (if I can be bothered).

```shell
go run main.go [URL to a Daily Mail article]

```

Some log lines will be written to stderr. If all goes well, a CSV of comments will be written to a file named
'[article name]-comments.csv'.

### Common problems
- JSON parsing error: headless browser pulled text before it was ready. Solution: increase timeout field in DefaultCfg.
(see comments for why automated browser tooling is involved here)

...



