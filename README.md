# Go Hardcoded Subtitle Translator

Can be used to detect and translate hardcoded subtitles / other text in video, and output them as a subtitle file (currently WEBVTT is supported). Also allows you to output a translated version instead.

OCR is done using Google's video intelligence, translation using Google Translate or Naver's Papago.

Mostly tested to detect Korean (using Hangul as script filter) and translating to english, but all languages should be supported. (As long as the translating engine choses supports the language, and google's ocr can read it).

To use, simply copy the `config.json.example` to `config.json` and fill the details. API keys for both Google and Naver are required (if using naver for translation).

## Config

The config looks like this, here comments are added to explain every value, to use the config simply copy the example config provided.
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
                "filterLanguage": true,
                "language": "Korean",
                // Determines if we give the OCR language hints
                "detectLanguage": false,
                // BCP-47 format for language hints
                "languageHints": ["ko-KR"]
            },
            "subtitleLocation": {
                // Determines if we filter out text not within a given box
                "restrictLocation": true,
                // Box size in %. Box size is from top to bottom, left to right, 0 to 100%
                "top": 80,
                "bottom": 100,
                "left": 0,
                "right": 100
            },
            // Confidence threshold, filter out any text with a confidence below this
            "confidence": 75
        },
        "translation": {
            // Determines if we translate the text or not
            "translate": true,
            // What translation service to use, currently supported: Google, Naver (Naver = Papago)
            "engine": "Google",
            // Languages in ISO-639-1 format
            "sourceLanguage": "ko",
            "targetLanguage": "en"
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
            "partialMatchPercentage": 50
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
