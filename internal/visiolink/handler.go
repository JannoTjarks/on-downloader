package visiolink

import (
	"encoding/json"
	"fmt"
	"io"
	"jannotjarks/on-downloader/internal/auth"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

func GetOstfriesischeNachrichtenMetadata() Paper {
	return Paper{
		CatalogId:    12968,
		Customer:     "ostfriesischenachrichten",
		Domain:       "epaper.on-online.de",
		LoginDomain:  "www.on-online.de",
		ReaderDomain: "reader.on-online.de",
	}
}

func GetNewestIssue(paper Paper, client *http.Client) Catalog {
	t := time.Now()
	year := fmt.Sprintf("%d", t.Year())
	month := fmt.Sprintf("%d", t.Month())

	issues := GetIssues(paper, client, year, month)
	return issues[len(issues)-1]
}

func GetSpecificIssue(paper Paper, client *http.Client, date string) Catalog {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		log.Fatal(err)
	}

	year := fmt.Sprintf("%d", t.Year())
	month := fmt.Sprintf("%d", t.Month())

	issues := GetIssues(paper, client, year, month)

	publicationDate := t.Format(time.DateOnly)
	fmt.Printf("Searching the issue from the following date: %s\n", publicationDate)

	var specificIssue Catalog
	for _, issue := range issues {
		if issue.PublicationDate == publicationDate {
			specificIssue = issue
			break
		}
	}

	if specificIssue.Catalog == 0 {
		log.Fatal("There was no issue published on this date!")
	}

	return specificIssue
}

func GetIssues(paper Paper, client *http.Client, year string, month string) []Catalog {
	endpoint := "http://device.e-pages.dk/content/desktop/available.php"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}

	q := req.URL.Query()
	q.Add("customer", paper.Customer)
	q.Add("folder_id", fmt.Sprintf("%d", paper.CatalogId))
	q.Add("year", year)
	q.Add("month", month)
	req.URL.RawQuery = q.Encode()

	fmt.Println(req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("The http status code is \"%s\"\n", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(body))
	var issues Content
	err = json.Unmarshal(body, &issues)
	if err != nil {
		log.Fatalln(err)
	}

	return issues.Catalogs
}

func GetLoginUrl(paper Paper, client *http.Client, creds auth.Credentials) (string, error) {
	endpoint := fmt.Sprintf("https://%s/benutzer/loginVisiolink", paper.LoginDomain)

	redirectUrl := fmt.Sprintf("https://%s/titles/%s/%d/?token=[OneTimeToken]", paper.Domain, paper.Customer, paper.CatalogId)
	form := url.Values{}
	form.Add("_method", "POST")
	form.Add("redirect-url", redirectUrl)
	form.Add("data[Benutzer][username]", creds.Username)
	form.Add("data[Benutzer][passwort]", creds.Password)
	form.Add("stay", "1")

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("The http status code is \"%s\"\n", resp.Status)
	fmt.Printf("The loginUrl is \"%s\"\n", resp.Request.URL.String())
	return resp.Request.URL.String(), nil
}

func ExtractSecretFromLoginUrl(paper Paper, client *http.Client, loginUrl string) (string, error) {
	urlPattern := fmt.Sprintf(regexp.QuoteMeta(fmt.Sprintf("https://%s/titles/%s/%d/publications/", paper.Domain, paper.Customer, paper.CatalogId)) + `(\d*)/\?secret=(.*)`)

	fmt.Println(urlPattern)
	re := regexp.MustCompile(urlPattern)

	matches := re.FindStringSubmatch(loginUrl)
	if len(matches) != 3 {
		log.Fatal("Secret extraction failed")
	}

	issue := matches[1]
	fmt.Printf("The extracted issue from the loginUrl is \"%s\"\n", issue)
	secret := matches[2]
	fmt.Printf("The extracted secret from the loginUrl is \"%s\"\n", secret)

	return secret, nil
}

func GetIssueAccessUrl(paper Paper, client *http.Client, secret string, newestIssueId int) (string, error) {
	endpoint := fmt.Sprintf("https://login-api.e-pages.dk/v1/%s/private/validate/prefix/%s/publication/%d/token", paper.Domain, paper.Customer, newestIssueId)

	data := url.Values{}
	data.Add("referrer_url", "POST")
	data.Add("token", secret)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("The http status code is \"%s\"\n", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(body))
	var accessUrl TokenResponse
	err = json.Unmarshal(body, &accessUrl)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("The access_url from the issue is \"%s\"\n", accessUrl.AccessURL)

	return accessUrl.AccessURL, nil
}

func GetIssueAccessKey(client *http.Client, accessUrl string) (string, error) {
	endpoint := accessUrl

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("The http status code is \"%s\"\n", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(body))
	re := regexp.MustCompile(`key4: '(.*)'`)

	matches := re.FindStringSubmatch(string(body))
	if len(matches) != 2 {
		log.Fatal("Key extraction failed")
	}

	accessKey := matches[1]

	fmt.Printf("The access_key for the issue is \"%s\"\n", accessKey)
	return accessKey, nil
}

func GenerateFileName(issue Catalog) string {
	return fmt.Sprintf("%s-%s.pdf", issue.Customer, issue.PublicationDate)
}

func DownloadIssue(paper Paper, client *http.Client, issueId int, accessKey string, fileName string) error {
	endpoint := fmt.Sprintf("https://front.e-pages.dk/session-cc/%s/%s/%d/pdf/download_pdf.php", accessKey, paper.Customer, issueId)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		log.Fatal(err)
	}

	q := req.URL.Query()
	q.Add("domain", paper.ReaderDomain)
	req.URL.RawQuery = q.Encode()

	fmt.Println(req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	fmt.Printf("The http status code is \"%s\"\n", resp.Status)

	out, fileErr := os.Create(fileName)
	if fileErr != nil {
		log.Fatal(fileErr)
	}
	defer out.Close()

	_, writeErr := io.Copy(out, resp.Body)
	if writeErr != nil {
		log.Fatal(writeErr)
	}

	return nil
}
