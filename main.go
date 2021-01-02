package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
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
	Translate      bool   `json:"translate"`
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
	Source       int64
}

type FormatVTT struct {
	Hours        string
	Minutes      string
	Seconds      string
	Milliseconds string
	Timecode     string
}

type Subtitle struct {
	start Duration
	end   Duration
	text  string
}

type PartialMatching struct {
	Match           bool
	MatchPercentage int
}

type FixOptions struct {
	IgnoreWhitespace bool
	Partial          PartialMatching
	MinimumDuration  int
}

type TranslateOptions struct {
	Client string
	Source string
	Target string
}

var cfg Config

var myClient = &http.Client{Timeout: 10 * time.Second}

func subsFromVideo(inputFile string) (subtitles map[string]*Subtitle, subtitlesKeys []string) {
	subtitles = make(map[string]*Subtitle)
	subtitlesKeys = make([]string, 0)

	ctx := context.Background()

	// Creates a client.
	client, err := video.NewClient(ctx, option.WithCredentialsFile(cfg.Google.APIKey))
	check(err)

	// Opens input file
	fileBytes, err := ioutil.ReadFile(inputFile)
	check(err)

	// Use google's video intelligence to do ocr
	op, err := client.AnnotateVideo(ctx, &videopb.AnnotateVideoRequest{
		Features: []videopb.Feature{
			videopb.Feature_TEXT_DETECTION,
		},
		InputContent: fileBytes,
	})
	check(err)

	resp, err := op.Wait(ctx)
	check(err)

	result := resp.GetAnnotationResults()[0]

	// Loop over results to filter and store subtitles
	for _, annotation := range result.TextAnnotations {
		text := annotation.GetText()
		info := whatlanggo.Detect(text)
		if whatlanggo.Scripts[info.Script] == cfg.Settings.Script {
			segment := annotation.GetSegments()[0]
			start, _ := ptypes.Duration(segment.GetSegment().GetStartTimeOffset())
			end, _ := ptypes.Duration(segment.GetSegment().GetEndTimeOffset())
			startDuration := parseTimecode(start.Milliseconds())
			endDuration := parseTimecode(end.Milliseconds())
			keys := strconv.FormatInt(start.Milliseconds(), 10)
			for len(keys) < 12 {
				keys = "0" + keys
			}

			subtitles[keys] = new(Subtitle)
			subtitles[keys].start = startDuration
			subtitles[keys].end = endDuration
			subtitles[keys].text = text

			subtitlesKeys = append(subtitlesKeys, keys)
		}
	}

	return
}

func parseTimecode(timecode int64) (parsed Duration) {
	parsed.Source = timecode
	parsed.Milliseconds = timecode % 1000
	parsed.Seconds = int64((timecode / 1000) % 60)
	parsed.Minutes = int64((timecode / (1000 * 60)) % 60)
	parsed.Hours = int64((timecode / (1000 * 60 * 60)) % 24)

	return
}

func naver(text string, source string, target string, targetStruct interface{}) error {
	body := strings.NewReader(`source=` + source + `&target=` + target + `&text=` + text)
	req, err := http.NewRequest("POST", cfg.Naver.Endpoint, body)
	check(err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Naver-Client-Id", cfg.Naver.ClientId)
	req.Header.Set("X-Naver-Client-Secret", cfg.Naver.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	check(err)

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(targetStruct)
}

func fixSubtitles(subtitles map[string]*Subtitle, subtitlesKeys []string, fixOptions FixOptions) (map[string]*Subtitle, []string) {
	sort.Strings(subtitlesKeys)

	// Fix duplicates
	for key := 0; key < len(subtitlesKeys); key++ {
		subKey := subtitlesKeys[key]

		if key+1 < len(subtitlesKeys) {
			if subtitles[subKey].text == subtitles[subtitlesKeys[key+1]].text {
				subtitles[subKey].end = subtitles[subtitlesKeys[key+1]].end
				subtitlesKeys = deleteFromSlice(subtitlesKeys, key+1)
				key--
			} else if fixOptions.IgnoreWhitespace && strings.ReplaceAll(subtitles[subKey].text, " ", "") == strings.ReplaceAll(subtitles[subtitlesKeys[key+1]].text, " ", "") {
				subtitles[subKey].end = subtitles[subtitlesKeys[key+1]].end
				subtitlesKeys = deleteFromSlice(subtitlesKeys, key+1)
				key--
			} else if fixOptions.Partial.Match {
				base := strings.Fields(subtitles[subKey].text)
				compare := strings.Fields(subtitles[subtitlesKeys[key+1]].text)
				match := 0

				for _, value := range base {
					i, success := find(compare, value)
					if success {
						deleteFromSlice(compare, i)
						match++
					}
				}

				perc := float64(match) / float64(len(base)) * float64(100)

				if perc > float64(fixOptions.Partial.MatchPercentage) {
					subtitles[subKey].end = subtitles[subtitlesKeys[key+1]].end
					subtitlesKeys = deleteFromSlice(subtitlesKeys, key+1)
					key--
				}
			}
		}
	}

	// Fix duration
	for index, value := range subtitlesKeys {
		subtitle := subtitles[value]
		if (subtitle.end.Source - subtitle.start.Source) < int64(fixOptions.MinimumDuration) {
			var newLength int64
			if index+1 < len(subtitlesKeys) {
				if (subtitles[subtitlesKeys[index+1]].start.Source - subtitle.start.Source) < int64(fixOptions.MinimumDuration) {
					newLength = subtitles[subtitlesKeys[index+1]].start.Source - subtitle.start.Source
				} else {
					newLength = int64(fixOptions.MinimumDuration)
				}
			} else {
				newLength = int64(fixOptions.MinimumDuration)
			}

			newEnd := parseTimecode(subtitle.start.Source + newLength)
			subtitles[value].end = newEnd
		}
	}

	return subtitles, subtitlesKeys
}

func find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

func translateSubtitles(subtitles map[string]*Subtitle, subtitlesKeys []string, translateOptions TranslateOptions) (map[string]*Subtitle, []string) {

	for _, value := range subtitlesKeys {
		if translateOptions.Client == "Naver" {
			translation := new(Translation)
			naver(subtitles[value].text, translateOptions.Source, translateOptions.Target, translation)
			subtitles[value].text = translation.Message.Result.TranslatedText
		}

	}

	return subtitles, subtitlesKeys
}

func writeToFile(subtitles map[string]*Subtitle, subtitlesKeys []string, format string) {
	for _, value := range subtitlesKeys {
		// Format
		var timecode string
		var text string
		if format == "vtt" {
			start := durationToVTT(subtitles[value].start)
			end := durationToVTT(subtitles[value].end)
			timecode = start.Timecode + " --> " + end.Timecode + "\n"
			text = subtitles[value].text + "\n\n"
		}

		// Write to file
		f, err := os.OpenFile(cfg.Settings.OutputFile,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		check(err)
		defer f.Close()

		if _, err := f.WriteString(timecode + text); err != nil {
			check(err)
		}
	}
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

func deleteFromSlice(arr []string, i int) []string {
	// Remove the element at index i from a.
	copy(arr[i:], arr[i+1:]) // Shift a[i+1:] left one index.
	arr[len(arr)-1] = ""     // Erase last element (write zero value).
	arr = arr[:len(arr)-1]   // Truncate slice.
	return arr
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	err := cleanenv.ReadConfig("config.json", &cfg)
	check(err)

	// 	1) 	Parse text from video, return map with key: start.Milliseconds()
	//    	and value Subtitle, also a slice for sorting
	subtitles, subtitlesKeys := subsFromVideo(cfg.Settings.InputFile)

	//	2)	Fix subtitles by merging duplicates (as long as they come after each other within x time)
	// TODO: Put these options in the cfg
	fixOptions := &FixOptions{}
	fixOptions.IgnoreWhitespace = true
	fixOptions.MinimumDuration = 2000
	fixOptions.Partial.Match = true
	fixOptions.Partial.MatchPercentage = 75
	subtitles, subtitlesKeys = fixSubtitles(subtitles, subtitlesKeys, *fixOptions)

	//	3)	Translate subtitles (language of choice, optional)
	if cfg.Settings.Translate {
		translateOptions := &TranslateOptions{"Naver", cfg.Settings.SourceLanguage, cfg.Settings.TargetLanguage}
		subtitles, subtitlesKeys = translateSubtitles(subtitles, subtitlesKeys, *translateOptions)
	}

	//	4)	Write subtitles to file (format of choice)
	writeToFile(subtitles, subtitlesKeys, "vtt")
}
