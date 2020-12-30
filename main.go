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

func textfromvid() {
	ctx := context.Background()

	// Creates a client.
	client, err := video.NewClient(ctx, option.WithCredentialsFile(cfg.Google.APIKey))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	fileBytes, err := ioutil.ReadFile(cfg.Settings.InputFile)
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
			startCode := convertTimecode(start.String())
			startCodeTrimmed := strings.Replace(startCode, ":", "", -1)
			startCodeTrimmed = strings.Replace(startCodeTrimmed, ".", "", -1)
			translation := new(Translation) // or &Foo{}

			getJson(text, cfg.Settings.SourceLanguage, cfg.Settings.TargetLanguage, translation)
			translatedText := translation.Message.Result.TranslatedText

			holder[startCodeTrimmed] = make(map[string]string)
			holder[startCodeTrimmed]["text"] = translatedText
			holder[startCodeTrimmed]["start"] = startCode
			holder[startCodeTrimmed]["end"] = convertTimecode(end.String())
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
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func convertTimecode(time string) string {
	arr := strings.Split(time, "h")
	if len(arr) != 2 {
		arr = append(arr, arr[0])
		arr[0] = "00"
	}
	arr = append(arr, strings.Split(arr[1], "m")...)
	arr = deleteFromSlice(arr, 1)
	if len(arr) != 3 {
		arr = append(arr, arr[1])
		arr[1] = "00"
	}

	arr = append(arr, strings.Split(arr[2], "s")...)
	arr = deleteFromSlice(arr, 2)
	if len(arr) != 4 {
		arr = append(arr, arr[2])
		arr[2] = "00"
	}
	arr = append(arr, strings.Split(arr[2], ".")...)

	arr = deleteFromSlice(arr, 2)
	if len(arr) != 5 {
		arr = append(arr, arr[2])
		arr[2] = "00"
	}
	arr = deleteFromSlice(arr, 2)
	for i, value := range arr {
		if len(value) < 2 && i != 3 {
			arr[i] = "0" + value
		}
		if i == 3 {
			arr[i] = arr[i] + "0"
			if len(arr[i]) < 3 {
				arr[i] = arr[i] + "0"
			}
			if len(arr[i]) < 3 {
				arr[i] = arr[i] + "0"
			}
		}
	}
	return arr[0] + ":" + arr[1] + ":" + arr[2] + "." + arr[3]
}

func deleteFromSlice(arr []string, i int) []string {
	// Remove the element at index i from a.
	copy(arr[i:], arr[i+1:]) // Shift a[i+1:] left one index.
	arr[len(arr)-1] = ""     // Erase last element (write zero value).
	arr = arr[:len(arr)-1]   // Truncate slice.
	return arr
}

func main() {
	err := cleanenv.ReadConfig("config.json", &cfg)
	if err != nil {
		panic(err)
	}
	textfromvid()
}
