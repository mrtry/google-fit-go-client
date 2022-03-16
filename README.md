# google-fit-go-client

- [Google Fit REST API](https://developers.google.com/fit/rest)のサンプル
- [fitness](https://pkg.go.dev/google.golang.org/api/fitness/v1) を使っている
- Google Fitから以下のデータを取得できる
  - 歩数
  - 睡眠時間
  - 体重
  - 安静時心拍数
  - 体温


```
$ go run main.go 
Step: 11324
Sleep: 08:08:30
Weight: 64.199997
Heart Rate: 60.000000
Body Temperature: 35.599998
```

## Setup

- Google Fitに「歩数」「睡眠時間」「体重」「安静時心拍数」「体温」を入れておく
- [コードの日付指定箇所](https://github.com/mrtry/google-fit-go-client/blob/main/main.go#L37)を変更して、Google Fitにデータが入ってる日付を指定する
- GCP上でFitness APIを有効にし、OAuth Clientを作っておく
- `.env` を作成し、以下の項目を埋める

```
CLIENT_ID=<GCPのOAuthClientのClient ID>
CLIENT_SECRET=<GCPのOAuthClientのSecret ID>
REDIRECT_URL=<GCPのOAuthClientに設定したRedirect URI>
```

## 使い方

### OAuthする

OAuthのTokenがない状態で実行すると以下のようになる

```
$ go run main.go 
go run main.go  
(*fs.PathError)(0xc0001cb590)(open .token-source.cache: no such file or directory)
redirect_url is empty.
Require authenticate.
AuthCodeURL: https://accounts.google.com/o/oauth2/auth...
```

表示されたAuthCodeURLにアクセスし、Redirect先のURLをコピーしておく

例として、GCPのOAuthClientに設定したRedirect URIが `http://localhost` の場合は、以下のようなURLになる
```
http://localhost/?state=state&code=...
```

得られたURLを引数として渡すと、OAuthのTokenを取得し、それを元にAPIへアクセスする


```
$ go run main.go -redirect_url=http://localhost/\?state\=state\&code\=4%2...
(*fs.PathError)(0xc0001a9590)(open .token-source.cache: no such file or directory)
Step: 11324
Sleep: 08:08:30
Weight: 64.199997
Heart Rate: 60.000000
Body Temperature: 35.599998
```

取得されたTokenは `.token-source.cache` に保存され、それ以降は`-redirect_url`の引数なしにAPIへアクセスできる

```
$ go run main.go 
Step: 11324
Sleep: 08:08:30
Weight: 64.199997
Heart Rate: 60.000000
Body Temperature: 35.599998
```
