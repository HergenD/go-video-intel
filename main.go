package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	video "cloud.google.com/go/videointelligence/apiv1"
	"github.com/abadojack/whatlanggo"
	"github.com/golang/protobuf/ptypes"
	"github.com/ilyakaznacheev/cleanenv"
	"google.golang.org/api/option"
	videopb "google.golang.org/genproto/googleapis/cloud/videointelligence/v1"
)

type Translation struct {
	Message Message
}
type Message struct {
	Type    string
	Service string
	Version string
	Result  Result
}
type Result struct {
	SrcLangType    string
	TarLangType    string
	TranslatedText string
	EngineType     string
}

type Config struct {
	Naver    NaverConfig    `json:"naver"`
	Google   GoogleConfig   `json:"google"`
	Settings SettingsConfig `json:"settings"`
}

type NaverConfig struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Endpoint     string `json:"endpoint"`
}

type GoogleConfig struct {
	APIKey string `json:"apiKey"`
}

type SettingsConfig struct {
	Script         string `json:"script"`
	SourceLanguage string `json:"sourceLanguage"`
	TargetLanguage string `json:"targetLanguage"`
	InputFile      string `json:"inputFile"`
	OutputFile     string `json:"outputFile"`
}

type Duration struct {
	Hours        int64
	Minutes      int64
	Seconds      int64
	Milliseconds int64
}

type FormatVTT struct {
	Hours        string
	Minutes      string
	Seconds      string
	Milliseconds string
	Timecode     string
}

var cfg Config

var myClient = &http.Client{Timeout: 10 * time.Second}

func getJson(text string, source string, target string, targetStruct interface{}) error {
	body := strings.NewReader(`source=` + source + `&target=` + target + `&text=` + text)
	req, err := http.NewRequest("POST", cfg.Naver.Endpoint, body)
	if err != nil {
		check(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Naver-Client-Id", cfg.Naver.ClientId)
	req.Header.Set("X-Naver-Client-Secret", cfg.Naver.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		check(err)
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(targetStruct)
}

func textfromvid(inputFile string) bool {
	ctx := context.Background()

	// Creates a client.
	client, err := video.NewClient(ctx, option.WithCredentialsFile(cfg.Google.APIKey))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fileBytes, err := ioutil.ReadFile(inputFile)
	if err != nil {
		fmt.Println(err)
	}

	op, err := client.AnnotateVideo(ctx, &videopb.AnnotateVideoRequest{
		Features: []videopb.Feature{
			videopb.Feature_TEXT_DETECTION,
		},
		InputContent: fileBytes,
	})
	if err != nil {
		// fmt.Errorf("AnnotateVideo: %v", err)
	}

	resp, err := op.Wait(ctx)
	if err != nil {
		// fmt.Errorf("Wait: %v", err)
	}
	result := resp.GetAnnotationResults()[0]
	holder := make(map[string]map[string]string)
	counter := make([]string, 0)
	for _, annotation := range result.TextAnnotations {
		text := annotation.GetText()
		info := whatlanggo.Detect(text)
		if whatlanggo.Scripts[info.Script] == "Hangul" {
			segment := annotation.GetSegments()[0]
			start, _ := ptypes.Duration(segment.GetSegment().GetStartTimeOffset())
			end, _ := ptypes.Duration(segment.GetSegment().GetEndTimeOffset())
			startCode := durationToVTT(parseTimecode(start.Milliseconds())).Timecode
			startCodeTrimmed := strings.Replace(startCode, ":", "", -1)
			startCodeTrimmed = strings.Replace(startCodeTrimmed, ".", "", -1)
			translation := new(Translation) // or &Foo{}

			getJson(text, cfg.Settings.SourceLanguage, cfg.Settings.TargetLanguage, translation)
			translatedText := translation.Message.Result.TranslatedText

			holder[startCodeTrimmed] = make(map[string]string)
			holder[startCodeTrimmed]["text"] = translatedText
			holder[startCodeTrimmed]["start"] = startCode
			holder[startCodeTrimmed]["end"] = durationToVTT(parseTimecode(end.Milliseconds())).Timecode
			counter = append(counter, startCodeTrimmed)
		}
	}

	sort.Strings(counter)

	for _, value := range counter {
		f, err := os.OpenFile(cfg.Settings.OutputFile,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			check(err)
		}
		defer f.Close()
		if _, err := f.WriteString(holder[value]["start"] + " --> " + holder[value]["end"] + "\n" + holder[value]["text"] + "\n\n"); err != nil {
			check(err)
		}
	}
	fmt.Println(cfg.Settings.OutputFile + " has been made.")
	return true
}

func parseTimecode(timecode int64) (parsed Duration) {
	parsed.Milliseconds = timecode % 1000
	parsed.Seconds = int64((timecode / 1000) % 60)
	parsed.Minutes = int64((timecode / (1000 * 60)) % 60)
	parsed.Hours = int64((timecode / (1000 * 60 * 60)) % 24)
	return
}

func durationToVTT(duration Duration) (vtt FormatVTT) {

	vtt.Hours = strconv.FormatInt(duration.Hours, 10)
	vtt.Minutes = strconv.FormatInt(duration.Minutes, 10)
	vtt.Seconds = strconv.FormatInt(duration.Seconds, 10)
	vtt.Milliseconds = strconv.FormatInt(duration.Milliseconds, 10)

	for len(vtt.Hours) < 2 {
		vtt.Hours = "0" + vtt.Hours
	}
	for len(vtt.Minutes) < 2 {
		vtt.Minutes = "0" + vtt.Minutes
	}
	for len(vtt.Seconds) < 2 {
		vtt.Seconds = "0" + vtt.Seconds
	}
	for len(vtt.Milliseconds) < 3 {
		vtt.Milliseconds = vtt.Milliseconds + "0"
	}

	vtt.Timecode = vtt.Hours + ":" + vtt.Minutes + ":" + vtt.Seconds + "." + vtt.Milliseconds

	return
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	err := cleanenv.ReadConfig("config.json", &cfg)
	if err != nil {
		panic(err)
	}
	text := textfromvid(cfg.Settings.InputFile)
	if text {
		fmt.Println("done")
	}
}
