# Go Hardcoded Subtitle Translator

-- Currently very much WIP --

Can be used to detect and translate hardcoded subtitles / other text in video, and output them as a subtitle file (currently WEBVTT is supported). Also allows you to output a translated version instead.

OCR is done using Google's video intelligence, translation using Naver's Papago.

Currently, use is limited to KO>EN translation due to Papago use, but all languages should be detectable without translation (not tested, WIP)

To use, simply copy the `config.json.example` to `config.json` and fill the details. API keys for both Google and Naver are required.

## Config

The config looks like this:
```json
{
    "naver": {
        "clientId": "id",
        "clientSecret": "secret",
        "endpoint": "https://openapi.naver.com/v1/papago/n2mt"
    },
    "google": {
        "apiKey": "path"
    },
    "settings": {
        "detection": {
            "language": {
                "filterScript": true,
                "script": "Hangul",
                "filterLanguage": false,
                "language": "Korean",
                "detectLanguage": false,
                "languageCode": "ko-KR"
            },
            "subtitleLocation": {
                "restrictLocation": true,
                "top": 60,
                "bottom": 100,
                "left": 0,
                "right": 100
            },
            "confidence": 90
        },
        "translation": {
            "translate": true,
            "engine": "Naver",
            "sourceLanguage": "ko-KR",
            "targetLanguage": "en-US"
        },
        "fixSubtitles": {
            "fix": true,
            "ignoreWhitespace": true,
            "minimumDuration": 2000,
            "partialMatch": true,
            "partialMatchPercentage": 75
        },        
        "inputFile": "video/input.mp4",
        "outputFile": "output/output.vtt"
    }
}
```
## Run
To build, simply
```bash
$ go build
```

Then to run:
```bash
$ ./go-video-intel
```
