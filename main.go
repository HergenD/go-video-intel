package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/translate"
	video "cloud.google.com/go/videointelligence/apiv1"
	"github.com/abadojack/whatlanggo"
	"github.com/golang/protobuf/ptypes"
	"github.com/ilyakaznacheev/cleanenv"
	"golang.org/x/text/language"
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
	InputFile    string             `json:"inputFile"`
	OutputFile   string             `json:"outputFile"`
	Detection    DetectionConfig    `json:"detection"`
	Translation  TranslationConfig  `json:"translation"`
	FixSubtitles FixSubtitlesConfig `json:"fixSubtitles"`
}

type DetectionConfig struct {
	Language         LanguageConfig         `json:"language"`
	SubtitleLocation SubtitleLocationConfig `json:"subtitleLocation"`
	Confidence       float32                `json:"confidence"`
}

type LanguageConfig struct {
	FilterScript   bool     `json:"filterScript"`
	Script         string   `json:"script"`
	FilterLanguage bool     `json:"filterLanguage"`
	Language       string   `json:"language"`
	DetectLanguage bool     `json:"detectLanguage"`
	LanguageHints  []string `json:"languageHints"`
}

type SubtitleLocationConfig struct {
	RestrictLocation bool    `json:"restrictLocation"`
	Top              float32 `json:"top"`
	Bottom           float32 `json:"bottom"`
	Left             float32 `json:"left"`
	Right            float32 `json:"right"`
}

type TranslationConfig struct {
	Translate      bool   `json:"translate"`
	Engine         string `json:"engine"`
	SourceLanguage string `json:"sourceLanguage"`
	TargetLanguage string `json:"targetLanguage"`
}

type FixSubtitlesConfig struct {
	Fix                    bool `json:"fix"`
	IgnoreWhitespace       bool `json:"ignoreWhitespace"`
	MinimumDuration        int  `json:"minimumDuration"`
	PartialMatch           bool `json:"partialMatch"`
	PartialMatchPercentage int  `json:"partialMatchPercentage"`
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
	Start      Duration
	End        Duration
	Text       string
	Vertices   []*videopb.NormalizedVertex
	Confidence float32
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
	settings := &videopb.AnnotateVideoRequest{
		Features: []videopb.Feature{
			videopb.Feature_TEXT_DETECTION,
		},
		InputContent: fileBytes,
	}
	if cfg.Settings.Detection.Language.DetectLanguage {
		settings.VideoContext = &videopb.VideoContext{
			TextDetectionConfig: &videopb.TextDetectionConfig{
				LanguageHints: cfg.Settings.Detection.Language.LanguageHints,
			},
		}
	}
	op, err := client.AnnotateVideo(ctx, settings)
	check(err)

	resp, err := op.Wait(ctx)
	check(err)

	result := resp.GetAnnotationResults()[0]
	fmt.Println("Total lines of text found:", len(result.TextAnnotations))
	// Loop over results to store subtitles
	for _, annotation := range result.TextAnnotations {
		text := annotation.GetText()
		for _, segment := range annotation.GetSegments() {
			confidence := segment.GetConfidence()
			frame := segment.GetFrames()[0]
			vertices := frame.GetRotatedBoundingBox().GetVertices()
			start, _ := ptypes.Duration(segment.GetSegment().GetStartTimeOffset())
			end, _ := ptypes.Duration(segment.GetSegment().GetEndTimeOffset())
			startDuration := parseTimecode(start.Milliseconds())
			endDuration := parseTimecode(end.Milliseconds())
			keys := strconv.FormatInt(start.Milliseconds(), 10)
			for len(keys) < 12 {
				keys = "0" + keys
			}
			_, found := find(subtitlesKeys, keys)
			if found {
				keys = keys + "a"
			}

			subtitles[keys] = new(Subtitle)
			subtitles[keys].Start = startDuration
			subtitles[keys].End = endDuration
			subtitles[keys].Text = text
			subtitles[keys].Vertices = vertices
			subtitles[keys].Confidence = confidence
			subtitlesKeys = append(subtitlesKeys, keys)
		}
	}

	sort.Strings(subtitlesKeys)

	return
}

func filterSubtitles(subtitles map[string]*Subtitle, subtitlesKeys []string) (map[string]*Subtitle, []string) {
	sort.Strings(subtitlesKeys)
	for i := 0; i < len(subtitlesKeys); i++ {
		key := subtitlesKeys[i]
		subtitle := subtitles[key]

		// Set filter counters, confidence is always filtered so start on 1
		passedFilters := 0
		filtersNeeded := 1
		info := whatlanggo.Detect(subtitle.Text)

		// Confidence filter
		if subtitle.Confidence*100 > cfg.Settings.Detection.Confidence {
			passedFilters++
		}
		// Script filter
		if cfg.Settings.Detection.Language.FilterScript {
			filtersNeeded++
			if whatlanggo.Scripts[info.Script] == cfg.Settings.Detection.Language.Script {
				passedFilters++
			}
		}
		// Language filter
		if cfg.Settings.Detection.Language.FilterLanguage {
			filtersNeeded++
			if info.Lang.String() == cfg.Settings.Detection.Language.Language {
				passedFilters++
			}
		}
		// Detection box filter
		sl := cfg.Settings.Detection.SubtitleLocation
		if sl.RestrictLocation {
			filtersNeeded++
			if // Filter top edge of box (vertices.Y 0 & 1)
			subtitle.Vertices[0].GetY()*100 > sl.Top &&
				subtitle.Vertices[1].GetY()*100 > sl.Top &&
				// Filter bottom edge of box (vertices.Y 2 & 3)
				subtitle.Vertices[2].GetY()*100 < sl.Bottom &&
				subtitle.Vertices[3].GetY()*100 < sl.Bottom &&
				// Filter left edge of box (vertices.X 0 & 2)
				subtitle.Vertices[0].GetX()*100 > sl.Left &&
				subtitle.Vertices[2].GetY()*100 > sl.Left &&
				// Filter right edge of box (vertices.X 1 & 3)
				subtitle.Vertices[1].GetY()*100 < sl.Right &&
				subtitle.Vertices[3].GetY()*100 < sl.Right {
			}
			passedFilters++
		}

		if filtersNeeded > passedFilters {
			subtitlesKeys = deleteFromSlice(subtitlesKeys, i)
			i--
		}
	}
	fmt.Println("Filtered text (subtitles) found:", len(subtitlesKeys))
	return subtitles, subtitlesKeys
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

func googleTranslate(text string, target string) (string, error) {
	ctx := context.Background()

	lang, err := language.Parse(target)
	check(err)

	client, err := translate.NewClient(ctx, option.WithCredentialsFile(cfg.Google.APIKey))
	check(err)
	defer client.Close()

	resp, err := client.Translate(ctx, []string{text}, lang, nil)
	check(err)

	if len(resp) == 0 {
		return "", fmt.Errorf("Translate returned empty response to text: %s", text)
	}

	return html.UnescapeString(resp[0].Text), nil
}

func fixDuplicates(subtitles map[string]*Subtitle, subtitlesKeys []string) (map[string]*Subtitle, []string) {
	// Fix duplicates
	mergeCount := 0
	mergeCountEasy := 0
	mergeCountWhitespace := 0
	mergeCountPartial := 0
	for key := 0; key < len(subtitlesKeys); key++ {
		subKey := subtitlesKeys[key]
		merge := false
		if key+1 < len(subtitlesKeys) {
			if subtitles[subKey].Text == subtitles[subtitlesKeys[key+1]].Text {
				merge = true
				mergeCountEasy++
			} else if cfg.Settings.FixSubtitles.IgnoreWhitespace && strings.ReplaceAll(subtitles[subKey].Text, " ", "") == strings.ReplaceAll(subtitles[subtitlesKeys[key+1]].Text, " ", "") {
				merge = true
				mergeCountWhitespace++
			} else if cfg.Settings.FixSubtitles.PartialMatch {
				base := strings.Fields(subtitles[subKey].Text)
				compare := strings.Fields(subtitles[subtitlesKeys[key+1]].Text)
				match := 0

				for _, value := range base {
					i, success := find(compare, value)
					if success {
						compare = deleteFromSlice(compare, i)
						match++
					}
				}

				perc := float64(match) / float64(len(base)) * float64(100)

				if perc > float64(cfg.Settings.FixSubtitles.PartialMatchPercentage) {
					merge = true
					mergeCountPartial++
				}
			}
		}

		// Merge subtitles based on confidence
		if merge {
			mergeCount++
			subtitles[subKey].End = subtitles[subtitlesKeys[key+1]].End
			if subtitles[subKey].Confidence < subtitles[subtitlesKeys[key+1]].Confidence {
				subtitles[subKey].Text = subtitles[subtitlesKeys[key+1]].Text
			}
			subtitlesKeys = deleteFromSlice(subtitlesKeys, key+1)
			key--
		}
	}
	fmt.Println("Subtitles merged:", mergeCount)
	fmt.Println("Of which", mergeCountEasy, "exact,", mergeCountWhitespace, "with whitespace removed and", mergeCountPartial, "partial matches")

	return subtitles, subtitlesKeys
}

func stackSubtitles(subtitles map[string]*Subtitle, subtitlesKeys []string) (map[string]*Subtitle, []string) {

	stackCount := 0
	for key := 0; key < len(subtitlesKeys); key++ {
		subKey := subtitlesKeys[key]
		stack := false
		if key+1 < len(subtitlesKeys) {
			if subtitles[subKey].Vertices[3].GetY() < subtitles[subtitlesKeys[key+1]].Vertices[0].GetY() ||
				subtitles[subKey].Vertices[0].GetY() > subtitles[subtitlesKeys[key+1]].Vertices[3].GetY() {
				if subtitles[subKey].Start.Source == subtitles[subtitlesKeys[key+1]].Start.Source {
					stack = true
				} else if subtitles[subKey].Start.Source > subtitles[subtitlesKeys[key+1]].Start.Source &&
					subtitles[subKey].End.Source < subtitles[subtitlesKeys[key+1]].Start.Source {
					stack = true
				}
			}
		}
		if stack {
			stackCount++
			if subtitles[subKey].Vertices[3].GetY() < subtitles[subtitlesKeys[key+1]].Vertices[0].GetY() {
				subtitles[subKey].Text = subtitles[subKey].Text + "\n" + subtitles[subtitlesKeys[key+1]].Text
			} else {
				subtitles[subKey].Text = subtitles[subtitlesKeys[key+1]].Text + "\n" + subtitles[subKey].Text
			}
			subtitlesKeys = deleteFromSlice(subtitlesKeys, key+1)
			key--
		}
	}
	fmt.Println("Subtitles stacked:", stackCount)
	return subtitles, subtitlesKeys
}

func fixDuration(subtitles map[string]*Subtitle, subtitlesKeys []string) (map[string]*Subtitle, []string) {
	for index, value := range subtitlesKeys {
		subtitle := subtitles[value]
		if (subtitle.End.Source - subtitle.Start.Source) < int64(cfg.Settings.FixSubtitles.MinimumDuration) {
			var newLength int64
			if index+1 < len(subtitlesKeys) {
				if (subtitles[subtitlesKeys[index+1]].Start.Source - subtitle.Start.Source) < int64(cfg.Settings.FixSubtitles.MinimumDuration) {
					newLength = subtitles[subtitlesKeys[index+1]].Start.Source - subtitle.Start.Source
				} else {
					newLength = int64(cfg.Settings.FixSubtitles.MinimumDuration)
				}
			} else {
				newLength = int64(cfg.Settings.FixSubtitles.MinimumDuration)
			}

			newEnd := parseTimecode(subtitle.Start.Source + newLength)
			subtitles[value].End = newEnd
		}
	}
	return subtitles, subtitlesKeys
}

func find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if strings.ToUpper(item) == strings.ToUpper(val) {
			return i, true
		}
	}
	return -1, false
}

func translateSubtitles(subtitles map[string]*Subtitle, subtitlesKeys []string) (map[string]*Subtitle, []string) {

	for _, value := range subtitlesKeys {
		if cfg.Settings.Translation.Engine == "Naver" {
			translation := new(Translation)
			naver(subtitles[value].Text, cfg.Settings.Translation.SourceLanguage, cfg.Settings.Translation.TargetLanguage, translation)
			subtitles[value].Text = translation.Message.Result.TranslatedText
		}
		if cfg.Settings.Translation.Engine == "Google" {
			subtitles[value].Text, _ = googleTranslate(subtitles[value].Text, cfg.Settings.Translation.TargetLanguage)
		}
	}
	fmt.Println(len(subtitlesKeys), "translated")
	return subtitles, subtitlesKeys
}

func writeToFile(subtitles map[string]*Subtitle, subtitlesKeys []string, format string) {
	f, err := os.Create(cfg.Settings.OutputFile)
	check(err)
	defer f.Close()

	if _, err := f.WriteString("WEBVTT"); err != nil {
		check(err)
	}

	for _, value := range subtitlesKeys {
		// Format
		var timecode string
		var text string
		if format == "vtt" {
			start := durationToVTT(subtitles[value].Start)
			end := durationToVTT(subtitles[value].End)
			timecode = "\n\n" + start.Timecode + " --> " + end.Timecode + "\n"
			text = subtitles[value].Text
		}

		// Write to file
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

	if _, err := os.Stat(cfg.Settings.InputFile); err == nil {
		fmt.Println("Start processing", cfg.Settings.InputFile)
	} else if os.IsNotExist(err) {
		fmt.Println("No input file found")
		os.Exit(1)
	} else {
		panic(err)
	}

	if _, err := os.Stat(cfg.Settings.OutputFile); err == nil {
		fmt.Println("Output file exists, aborting.")
		os.Exit(1)
	} else if os.IsNotExist(err) {
		fmt.Println("Output will be written to:", cfg.Settings.OutputFile)
	} else {
		panic(err)
	}

	// 	1) 	Parse text from video, return map with key: start.Milliseconds()
	//    	and value Subtitle, also a slice for sorting
	fmt.Println("Doing OCR through Google API")
	subtitles, subtitlesKeys := subsFromVideo(cfg.Settings.InputFile)

	//	2)	Filter ubtitles based on config settings
	subtitles, subtitlesKeys = filterSubtitles(subtitles, subtitlesKeys)

	//	3)	Fix subtitles by merging duplicates (as long as they come after each other within x time)
	if cfg.Settings.FixSubtitles.Fix {
		fmt.Println("Fixing subtitles")
		subtitles, subtitlesKeys = fixDuplicates(subtitles, subtitlesKeys)
	}

	//	4)	Translate subtitles (language of choice, optional)
	if cfg.Settings.Translation.Translate {
		fmt.Println("Translating subtitles using", cfg.Settings.Translation.Engine)
		subtitles, subtitlesKeys = translateSubtitles(subtitles, subtitlesKeys)
		subtitles, subtitlesKeys = fixDuplicates(subtitles, subtitlesKeys)
	}

	subtitles, subtitlesKeys = stackSubtitles(subtitles, subtitlesKeys)
	subtitles, subtitlesKeys = fixDuration(subtitles, subtitlesKeys)

	fmt.Println("Writing to file")
	//	5)	Write subtitles to file (format of choice)
	writeToFile(subtitles, subtitlesKeys, "vtt")

	fmt.Println("Done")
}
