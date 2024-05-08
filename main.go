package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

var (
	optSourceProfile string
	optDuration      time.Duration
)

func init() {
	flag.StringVar(&optSourceProfile, "source-profle", "default", "switch source profile")
	flag.DurationVar(&optDuration, "d", time.Hour, "duration seconds for session expire")
	flag.Parse()
}

func main() {
	if len(flag.Args()) != 1 {
		log.Fatal("required profile")
	}
	targetProfile := flag.Args()[0]

	ctx := context.Background()
	// load target profile from ~/.aws/config
	cnf, err := config.LoadSharedConfigProfile(ctx, targetProfile)
	if err != nil {
		log.Fatal(err)
	}

	// check role arn
	roleArn, err := arn.Parse(cnf.RoleARN)
	if err != nil {
		log.Fatal(err)
	}
	if roleArn.Service != "iam" && roleArn.Resource != "role" {
		log.Fatal("invalid role_arn in config")
	}
	role := cnf.RoleARN

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithSharedConfigProfile(optSourceProfile), // always set source profile, because current shell exported temporary credentials.
	)
	if err != nil {
		log.Fatal(err)
	}
	stsCli := sts.NewFromConfig(cfg)

	sessionName := "via-msk"
	expireIn := int32(optDuration / time.Second)
	in := &sts.AssumeRoleInput{
		RoleArn:         aws.String(role),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int32(expireIn),
	}

	if cnf.MFASerial != "" {
		// check mfa arn
		mfaArn, err := arn.Parse(cnf.MFASerial)
		if err != nil {
			log.Fatal(err)
		}
		if mfaArn.Service != "iam" && mfaArn.Resource != "mfa" {
			log.Fatal("invalid role_arn in config")
		}

		// read MFA token from terminal
		mfaNum, err := readTokenCode()
		if err != nil {
			log.Fatal(err)
		}
		in.SerialNumber = aws.String(cnf.MFASerial)
		in.TokenCode = aws.String(mfaNum)
	}

	resp, err := stsCli.AssumeRole(ctx, in)
	if err != nil {
		log.Fatal(err)
	}

	AwsKey := *resp.Credentials.AccessKeyId
	AwsSecret := *resp.Credentials.SecretAccessKey
	AwsSessionToken := *resp.Credentials.SessionToken
	assumedRole := *resp.AssumedRoleUser.Arn
	expire := resp.Credentials.Expiration.Format(time.RFC3339)

	fmt.Printf("export AWS_ACCESS_KEY_ID=\"%s\"\n", AwsKey)
	fmt.Printf("export AWS_SECRET_ACCESS_KEY=\"%s\"\n", AwsSecret)
	fmt.Printf("export AWS_SESSION_TOKEN=\"%s\"\n", AwsSessionToken)
	fmt.Printf("export AWS_SECURITY_TOKEN=\"%s\"\n", AwsSessionToken)
	fmt.Printf("export ASSUMED_ROLE=\"%s\"\n", assumedRole)
	fmt.Printf("export AWS_PROFILE=\"%s\"\n", targetProfile)
	fmt.Printf("# this temporary credentials expire at %s\n", expire)
}

func readTokenCode() (string, error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "MFA code: ")
	mfaCode, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(mfaCode), nil
}
