package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/fitness/v1"
	"google.golang.org/api/option"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	CacheFileName   = ".token-source.cache"
	KeyAccessToken  = "KEY_ACCESS_TOKEN"
	KeyRefreshToken = "KEY_REFRESH_TOKEN"
	KeyExpiry       = "KEY_EXPIRY"
	KeyTokenType    = "KEY_TOKEN_TYPE"
	KeyClientId     = "CLIENT_ID"
	KeyClientSecret = "CLIENT_SECRET"
	KeyRedirectUrl  = "REDIRECT_URL"

	TimeFormatLayout = "2006-01-02T15:04:05Z07:00"
)

func main() {
	// Google Fitから取得したい日付を指定する
	location, _ := time.LoadLocation("Asia/Tokyo")
	date := time.Date(2022, 3, 6, 0, 0, 0, 0, location)

	ctx := context.Background()
	config, err := getOauthConfig()
	if err != nil {
		spew.Dump(err)
		return
	}

	tokenSource, err := restoreTokenSource(ctx, config)
	if err != nil {
		spew.Dump(err)

		// tokenがrestoreできないときは、引数のredirect urlからのcodeを用いてtokenを発行する
		var redirectUrl string
		flag.StringVar(&redirectUrl, "redirect_url", "", "OAuth後のredirectされたURL")
		flag.Parse()

		if redirectUrl == "" {
			authCodeUrl := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
			fmt.Printf("redirect_url is empty.\nRequire authenticate.\nAuthCodeURL: %s", authCodeUrl)
			return
		}
		ts, err := getTokenSourceFromRedirectUrl(ctx, *config, redirectUrl)
		if err != nil {
			spew.Dump(err)
			return
		}
		tokenSource = ts
	}

	client := oauth2.NewClient(ctx, tokenSource)
	fitnessService, err := createFitnessService(ctx, client)
	if err != nil {
		spew.Dump(err)
		return
	}

	stepCount, err := getStepCountByDate(fitnessService, date)
	if err != nil {
		spew.Dump(err)
		return
	}
	fmt.Printf("Step: %d\n", *stepCount)

	sleep, err := getSleepByDate(fitnessService, date)
	if err != nil {
		spew.Dump(err)
		return
	}
	fmt.Printf("Sleep: %s\n", time.Time{}.Add(*sleep).Format("15:04:05"))

	weight, err := getWeightByDate(fitnessService, date)
	if err != nil {
		spew.Dump(err)
		return
	}
	fmt.Printf("Weight: %f\n", *weight)

	heartRate, err := getHeartRateByDate(fitnessService, date)
	if err != nil {
		spew.Dump(err)
		return
	}
	fmt.Printf("Heart Rate: %f\n", *heartRate)

	temp, err := getBodyTemperatureByDate(fitnessService, date)
	if err != nil {
		spew.Dump(err)
		return
	}
	fmt.Printf("Body Temperature: %f\n", *temp)

	err = saveTokenSource(tokenSource)
	if err != nil {
		spew.Dump(err)
		return
	}
}

func getOauthConfig() (*oauth2.Config, error) {
	err := godotenv.Load(".env")
	if err != nil {
		return nil, err
	}

	clientId := os.Getenv(KeyClientId)
	clientSecret := os.Getenv(KeyClientSecret)
	redirectUrl := os.Getenv(KeyRedirectUrl)

	config := &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectUrl,
		Scopes: []string{
			fitness.FitnessActivityReadScope,
			fitness.FitnessBloodGlucoseReadScope,
			fitness.FitnessBloodPressureReadScope,
			fitness.FitnessBodyReadScope,
			fitness.FitnessHeartRateReadScope,
			fitness.FitnessBodyTemperatureReadScope,
			fitness.FitnessLocationReadScope,
			fitness.FitnessNutritionReadScope,
			fitness.FitnessOxygenSaturationReadScope,
			fitness.FitnessReproductiveHealthReadScope,
			fitness.FitnessSleepReadScope,
		},
		Endpoint: google.Endpoint,
	}

	return config, nil
}

func saveTokenSource(tokenSource oauth2.TokenSource) error {
	token, err := refreshTokenSourceIfNeeded(tokenSource)
	if err != nil {
		return err
	}

	// dotenvにTokenの内容を保存
	env, err := godotenv.Unmarshal(fmt.Sprintf("%s=%s\n%s=%s\n%s=%s\n%s=%s\n", KeyAccessToken, token.AccessToken, KeyRefreshToken, token.RefreshToken, KeyTokenType, token.TokenType, KeyExpiry, strconv.FormatInt(token.Expiry.Unix(), 10)))
	if err != nil {
		return err
	}

	err = godotenv.Write(env, CacheFileName)
	return err
}

func refreshTokenSourceIfNeeded(tokenSource oauth2.TokenSource) (*oauth2.Token, error) {
	// TokenSource returns a TokenSource that returns t until t expires, automatically refreshing it as necessary using the provided context.
	// see: https://pkg.go.dev/golang.org/x/oauth2#Config.TokenSource
	refreshedToken, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}

	return refreshedToken, nil
}

func restoreTokenSource(ctx context.Context, config *oauth2.Config) (oauth2.TokenSource, error) {
	err := godotenv.Load(CacheFileName)
	if err != nil {
		return nil, err
	}

	accessToken := os.Getenv(KeyAccessToken)
	refreshToken := os.Getenv(KeyRefreshToken)
	tokenType := os.Getenv(KeyTokenType)
	expiryAsUnix, err := strconv.ParseInt(os.Getenv(KeyExpiry), 10, 64)

	if err != nil {
		return nil, err
	}
	if accessToken == "" || refreshToken == "" || tokenType == "" || expiryAsUnix == 0 {
		return nil, errors.New(fmt.Sprintf("Failed to restore TokenSource.\naccessToken: %s\nrefreshToken: %s\n, tokenType: %s, expiryAsUnix: %d\n", accessToken, refreshToken, tokenType, expiryAsUnix))
	}

	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    tokenType,
		Expiry:       time.Unix(expiryAsUnix, 0),
	}

	tokenSource := config.TokenSource(ctx, token)
	return tokenSource, nil
}

func getTokenSourceFromRedirectUrl(ctx context.Context, config oauth2.Config, authUrl string) (oauth2.TokenSource, error) {
	parsed, err := url.Parse(authUrl)
	if err != nil {
		return nil, err
	}
	result, ok := parsed.Query()["code"]
	if !ok {
		authCodeUrl := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
		return nil, errors.New(fmt.Sprintf("authToken is empty.\nRequire authenticate.\nAuthCodeURL: %s", authCodeUrl))
	}
	authToken := result[0]

	if authToken == "" {
		authCodeUrl := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
		return nil, errors.New(fmt.Sprintf("Require authenticate.\nAuthCodeURL: %s", authCodeUrl))
	}

	token, err := config.Exchange(ctx, authToken)
	if err != nil {
		spew.Dump(err)
		authCodeUrl := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
		return nil, errors.New(fmt.Sprintf("Require authenticate.\nAuthCodeURL: %s", authCodeUrl))
	}

	return config.TokenSource(ctx, token), nil
}

func createFitnessService(ctx context.Context, client *http.Client) (*fitness.Service, error) {
	fitnessService, err := fitness.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	return fitnessService, nil
}

func getStepCountByDate(fitnessService *fitness.Service, time time.Time) (*int64, error) {
	request := fitness.AggregateRequest{
		AggregateBy: []*fitness.AggregateBy{
			{
				DataTypeName: "com.google.step_count.delta",
			},
		},
		StartTimeMillis: time.UnixMilli(),
		EndTimeMillis:   time.AddDate(0, 0, 1).UnixMilli(),
	}
	result, err := fitnessService.Users.Dataset.Aggregate("me", &request).Do()
	if err != nil {
		return nil, err
	}

	var steps int64 = 0
	for _, point := range result.Bucket[0].Dataset[0].Point {
		steps += point.Value[0].IntVal
	}

	return &steps, nil
}

func getSleepByDate(fitnessService *fitness.Service, date time.Time) (*time.Duration, error) {
	startTimeString := date.Format(TimeFormatLayout)
	endTimeString := date.AddDate(0, 0, 1).Format(TimeFormatLayout)

	// see: https://developers.google.com/fit/scenarios/read-sleep-data
	result, err := fitnessService.Users.Sessions.List("me").StartTime(startTimeString).EndTime(endTimeString).ActivityType(72).Do()

	if err != nil {
		return nil, err
	}

	// https://developers.google.com/fit/rest/v1/reference/users/sessions/list
	session := result.Session[0]
	asleep := time.UnixMilli(session.StartTimeMillis)
	awake := time.UnixMilli(session.EndTimeMillis)
	diff := awake.Sub(asleep)

	return &diff, nil
}

func getWeightByDate(fitnessService *fitness.Service, time time.Time) (*float64, error) {
	// see: https://developers.google.com/fit/datatypes/body
	request := fitness.AggregateRequest{
		AggregateBy: []*fitness.AggregateBy{
			{
				DataTypeName: "com.google.weight",
			},
		},
		StartTimeMillis: time.UnixMilli(),
		EndTimeMillis:   time.AddDate(0, 0, 5).UnixMilli(),
	}
	result, err := fitnessService.Users.Dataset.Aggregate("me", &request).Do()
	if err != nil {
		return nil, err
	}

	weight := result.Bucket[0].Dataset[0].Point[0].Value[0].FpVal

	return &weight, nil
}

func getHeartRateByDate(fitnessService *fitness.Service, time time.Time) (*float64, error) {
	// see: https://developers.google.com/fit/datatypes/body#heart_rate
	request := fitness.AggregateRequest{
		AggregateBy: []*fitness.AggregateBy{
			{
				DataTypeName: "com.google.heart_rate.bpm",
			},
		},
		StartTimeMillis: time.UnixMilli(),
		EndTimeMillis:   time.AddDate(0, 0, 5).UnixMilli(),
	}
	result, err := fitnessService.Users.Dataset.Aggregate("me", &request).Do()
	if err != nil {
		return nil, err
	}

	heartRate := result.Bucket[0].Dataset[0].Point[0].Value[0].FpVal

	return &heartRate, nil
}

func getBodyTemperatureByDate(fitnessService *fitness.Service, time time.Time) (*float64, error) {
	// see: https://developers.google.com/fit/datatypes/health#body_temperature
	request := fitness.AggregateRequest{
		AggregateBy: []*fitness.AggregateBy{
			{
				DataTypeName: "com.google.body.temperature",
			},
		},
		StartTimeMillis: time.UnixMilli(),
		EndTimeMillis:   time.AddDate(0, 0, 5).UnixMilli(),
	}
	result, err := fitnessService.Users.Dataset.Aggregate("me", &request).Do()
	if err != nil {
		return nil, err
	}

	temp := result.Bucket[0].Dataset[0].Point[0].Value[0].FpVal

	return &temp, nil
}
