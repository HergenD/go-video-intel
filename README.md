# Go Hardcoded Subtitle Translator

-- Currently very much WIP --

Can be used to detect and translate hardcoded subtitles / other text in video, and output them as a subtitle file (currently WEBVTT is supported). Also allows you to output a translated version instead.

OCR is done using Google's video intelligence, translation using Naver's Papago.

Currently, use is limited to KO>EN translation due to Papago use and detection limited to Hangul, although more options (and all languages/scripts) will be supported.

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
        "script": "Hangul",
        "sourceLanguage": "ko",
        "targetLanguage": "en",
        "inputFile": "input.mp4",
        "outputFile": "output.vtt",
        "translate": true
    }
}
```
## Run
To build, simply
`go build`

Then to run:
`./go-video-intel`