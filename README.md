# Go Hardcoded Subtitle Translator

-- Currently very much WIP --

Can be used to detect and translate hardcoded subtitles / other text in video, and output them as a subtitle file (currently WEBVTT is supported). Also allows you to output a translated version instead.

OCR is done using Google's video intelligence, translation using Naver's Papago.

Currently, use is limited to KO>EN translation due to Papago use, but all languages should be detectable without translation (not tested, WIP)

To use, simply copy the `config.json.example` to `config.json` and fill the details. API keys for both Google and Naver are required.

## Config

The config looks like this:
```js
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
                // Determines if text is filtered based on script
                "filterScript": true,
                "script": "Hangul",
                // Determines if text is filtered based on language
                "filterLanguage": false,
                "language": "Korean",
                // Determines if we give the OCR a language hint
                "detectLanguage": false,
                // BCP-47 format for language hint
                "languageCode": "ko-KR"
            },
            "subtitleLocation": {
                // Determines if we filter out text not within a given box
                "restrictLocation": true,
                // Box size in %. Box size is from top to bottom, left to right, 0 to 100%
                "top": 60,
                "bottom": 100,
                "left": 0,
                "right": 100
            },
            // Confidence threshold, filter out any text with a confidence below this (around 90 recommended)
            "confidence": 90
        },
        "translation": {
            // Determines if we translate the text or not
            "translate": true,
            // What translation service to use, currently supported: Naver
            "engine": "Naver",
            // Languages in BCP-47 format, currently supported: ko-KR, en-US, en-GB
            "sourceLanguage": "ko-KR",
            "targetLanguage": "en-US"
        },
        "fixSubtitles": {
            // Determines if we attempt to do some fixes on the subtitles (highly recommended)
            "fix": true,
            // Determines if whitespace should be take in consideration to match duplicate text (ignore = recommended)
            "ignoreWhitespace": true,
            // Determines minimum subtitle duration, although duration could be shorter if next 
            // subtitle starts before this minimum amount (in milliseconds)
            "minimumDuration": 2000,
            // Determines if duplicate text can be matched based on a x% of words matching
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
