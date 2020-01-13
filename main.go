package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"s3fc/base"
	"s3fc/boltdb"
	"s3fc/commands"
	"s3fc/inventory"
	"s3fc/logging"
	"s3fc/queries"
	selfS3 "s3fc/s3"
	"strings"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/google/uuid"

	"github.com/sirupsen/logrus"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"github.com/boltdb/bolt"
)

const (
	sessionName     = "s3fc"
	sessionDuration = 900
)

var (
	errInvalidRequest = errors.New("Invalid request, operation could not be determined")
)

// S3CatEvent is the input for requests.
type S3CatEvent struct {
	BoltDBURL  string  `json:"bolt_db_url"`
	AssumeRole string  `json:"assume_role"`
	ExternalID *string `json:"external_id"`

	LoadInventory          *commands.LoadInventory          `json:"load_inventory,omitempty"`
	PlanNewObjects         *commands.PlanNewObjects         `json:"plan_new_objects,omitempty"`
	PutObjectSet           *commands.PutObjectSet           `json:"put_object_set,omitempty"`
	TakeInventory          *commands.TakeInventory          `json:"take_inventory,omitempty"`
	UpdateObjectsState     *commands.UpdateObjectsState     `json:"update_object_state,omitempty"`
	WriteDestinationObject *commands.WriteDestinationObject `json:"write_destination_object,omitempty"`

	ListObjectByState *queries.ListObjectByState `json:"list_objects_by_state,omitempty"`
	GetSourceStats    *queries.GetSourceStats    `json:"get_source_stats,omitempty"`
}

// S3CatOutput is the output of responses.
type S3CatOutput struct {
	ListObjectByStateOutput *queries.ListObjectByStateOutput `json:"list_objects_by_state,omitempty"`
	GetSourceStatsOutput    *queries.GetSourceStatsOutput    `json:"get_source_stats,omitempty"`
}

// S3CatOutputHandler takes requests, routes to command or query and return a
// response.
type S3CatOutputHandler struct {
	s3Client  s3iface.S3API
	stsClient stsiface.STSAPI
}

// HandleRequest handles a request
func (s *S3CatOutputHandler) HandleRequest(ctx context.Context, event S3CatEvent) (*S3CatOutput, error) {
	var (
		output      S3CatOutput
		queryOutput interface{}
		action      interface{}
	)

	logger := logging.NewEventLogger(ctx, log)

	switch {
	case event.LoadInventory != nil:
		action = event.LoadInventory
	case event.PlanNewObjects != nil:
		action = event.PlanNewObjects
	case event.PutObjectSet != nil:
		action = event.PutObjectSet
	case event.TakeInventory != nil:
		action = event.TakeInventory
	case event.UpdateObjectsState != nil:
		action = event.UpdateObjectsState
	case event.WriteDestinationObject != nil:
		action = event.WriteDestinationObject
	case event.ListObjectByState != nil:
		action = event.ListObjectByState
		output.ListObjectByStateOutput = new(queries.ListObjectByStateOutput)
		queryOutput = output.ListObjectByStateOutput
	case event.GetSourceStats != nil:
		action = event.GetSourceStats
		output.GetSourceStatsOutput = new(queries.GetSourceStatsOutput)
		queryOutput = output.GetSourceStatsOutput
	default:
		logger.WithError(errInvalidRequest).Errorf("error parsing request")
		return nil, errInvalidRequest
	}

	logger = logger.WithField("action", fmt.Sprintf("%T", action))
	c := &lambdaContainer{
		// handler configuration items
		s3Client:  s.s3Client,
		stsClient: s.stsClient,
		// request configuration items
		ctx:        ctx,
		logger:     logger,
		dbURL:      event.BoltDBURL,
		assumeRole: event.AssumeRole,
		externalID: event.ExternalID,
	}
	defer func() {
		logger.Info("Starting Teardown")
		err := c.Close()
		logger.WithError(err).Info("Completed Teadown")
	}()

	if d, ok := action.(base.Dependent); ok {
		if err := d.Dependencies(c); err != nil {
			logger.WithError(err).Errorf("error injecting dependencies")
			return nil, err
		}
	}

	switch v := action.(type) {
	case base.Command:
		logger.Info("running command")
		if err := v.Invoke(ctx); err != nil {
			logger.WithError(err).Errorf("error invoking command")
			return nil, err
		}
	case base.Query:
		logger.Info("running query")
		var buf bytes.Buffer
		if err := v.Invoke(ctx, &buf); err != nil {
			logger.WithError(err).Errorf("error invoking query")
			return nil, err
		}

		if err := json.Unmarshal(buf.Bytes(), queryOutput); err != nil {
			logger.WithError(err).Errorf("error decoding query response")
			return nil, err
		}
	default:
		return nil, fmt.Errorf("(%T) Request is not a command or a query", action)

	}

	return &output, nil
}

// HandlerFunc returns a lambda handler function. This gives the opportunity
// to write a little wrapping code outside of the main handler function
func (s *S3CatOutputHandler) HandlerFunc() func(context.Context, S3CatEvent) (*S3CatOutput, error) {
	return func(ctx context.Context, event S3CatEvent) (*S3CatOutput, error) {
		return s.HandleRequest(ctx, event)
	}
}

var log logrus.FieldLogger
var level logrus.Level

func init() {
	level = logrus.DebugLevel

	l := os.Getenv("LOG_LEVEL")
	if l != "" {
		l, err := logrus.ParseLevel(l)
		if err != nil {
			level = l
		}
	}
	log = logging.New()
	log.(*logrus.Entry).Logger.SetLevel(level)
	log.(*logrus.Entry).Level = level
}

func main() {
	log.Info("cold start, level: ", level)

	s := session.Must(session.NewSession())

	s3Client := s3.New(s)
	stsClient := sts.New(s)

	xray.AWS(stsClient.Client)

	handler := &S3CatOutputHandler{
		s3Client:  s3Client,
		stsClient: stsClient,
	}
	lambda.Start(handler.HandlerFunc())
}

type lambdaContainer struct {
	// handler configuration items
	s3Client  s3iface.S3API
	stsClient stsiface.STSAPI

	// request configuration items
	ctx        context.Context
	logger     logrus.FieldLogger
	dbURL      string
	assumeRole string
	externalID *string

	// laziliy loaded components
	db           *bolt.DB
	inventory    base.InventoryManager
	requestS3API s3iface.S3API

	tearDowns []func() error
}

func (l *lambdaContainer) Logger() logrus.FieldLogger {
	return l.logger
}

func (l *lambdaContainer) InventoryManager() base.InventoryManager {
	if l.inventory != nil {
		return l.inventory
	}

	l.inventory = inventory.New(l.s3Client, l.Logger())

	return l.inventory
}

func (l *lambdaContainer) S3API() (s3iface.S3API, error) {
	if l.requestS3API != nil {
		return l.requestS3API, nil
	}

	if l.assumeRole == "" {
		l.requestS3API = l.s3Client
		return l.requestS3API, nil
	}

	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(l.assumeRole),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int64(sessionDuration),
		ExternalId:      l.externalID,
	}
	l.logger.WithFields(logrus.Fields{
		"role_arn":          aws.StringValue(assumeRoleInput.RoleArn),
		"role_session_name": aws.StringValue(assumeRoleInput.RoleSessionName),
		"duraiton_seconds":  aws.Int64Value(assumeRoleInput.DurationSeconds),
		"external_id":       aws.StringValue(assumeRoleInput.ExternalId),
	}).Debug("assuming request role")

	resp, err := l.stsClient.AssumeRoleWithContext(l.ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("Problem assuming role: %v", err)
	}

	s3Client := l.s3Client.(*s3.S3)
	sess, err := session.NewSession(&aws.Config{
		Region: s3Client.Config.Region,
		Credentials: credentials.NewStaticCredentials(
			*resp.Credentials.AccessKeyId,
			*resp.Credentials.SecretAccessKey,
			*resp.Credentials.SessionToken,
		),
	})
	if err != nil {
		return nil, err
	}

	client := s3.New(sess)

	l.requestS3API = client
	return l.requestS3API, nil
}

func (l *lambdaContainer) DB() (*bolt.DB, error) {
	if l.db != nil {
		return l.db, nil
	}

	db, err := openDatabase(l.ctx, l.s3Client, l.Logger(), l.dbURL)
	if err != nil {
		return nil, err
	}

	l.tearDowns = append(l.tearDowns, func() error {
		l.Logger().Info("Running DB Teardown")
		err := closeDatabase(l.ctx, l.s3Client, l.Logger(), l.dbURL, db)
		if err != nil {
			return fmt.Errorf("Problem uploading databse: %v", err)
		}
		return nil
	})

	l.db = db
	return db, nil
}

func (l *lambdaContainer) Close() error {
	var errs []interface{}
	for _, t := range l.tearDowns {
		if err := t(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) < 1 {
		return nil
	}

	return errors.New(fmt.Sprint(errs...))
}

func openDatabase(
	ctx context.Context,
	client s3iface.S3API,
	logger logrus.FieldLogger,
	rawURL string,
) (*bolt.DB, error) {
	bucket, key, err := parseBucketKey(rawURL)
	if err != nil {
		return nil, err
	}

	name := path.Join(os.TempDir(), uuid.Must(uuid.NewRandom()).String())
	w, err := os.Create(name)
	if err != nil {
		return nil, err
	}

	logger.WithFields(logrus.Fields{
		"bucket": bucket,
		"key":    key,
	}).Debug("downloading database.")
	if err = selfS3.DownloadObject(ctx, client, bucket, key, w); err != nil {
		w.Close()
		os.Remove(name)
		if selfS3.IsNotFound(err) {
			logger.Info("DB not found, creating a new one")
			return bolt.Open(name, 0600, nil)
		}
		return nil, fmt.Errorf("Problem downloading databse: %v", err)
	}

	if err = w.Close(); err != nil {
		os.Remove(name)
		return nil, err
	}

	return bolt.Open(name, 0600, nil)
}

func closeDatabase(
	ctx context.Context,
	client s3iface.S3API,
	logger logrus.FieldLogger,
	rawURL string,
	db *bolt.DB,
) error {
	defer closeAndDeleteDBFile(logger, db)

	bucket, key, err := parseBucketKey(rawURL)
	if err != nil {
		return err
	}

	r := boltdb.Backup(db)
	return selfS3.CreateObject(
		ctx, client, bucket, key, r,
	)
}

func closeAndDeleteDBFile(
	logger logrus.FieldLogger,
	db *bolt.DB,
) error {
	name := db.Path()
	if err := db.Close(); err != nil {
		log.WithError(err).Warn("problem closing database")
	}
	if err := os.Remove(name); err != nil {
		log.WithError(err).Warn("problem deleting database file")
	}
	logger.WithField("name", name).Debug("deleted bolt database")
	return nil
}

func parseBucketKey(rawURL string) (string, string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}

	if parsedURL.Scheme != "s3" {
		return "", "", errors.New("Invalid Scheme: " + parsedURL.Scheme)
	}

	bucket := parsedURL.Hostname()
	key := strings.Trim(parsedURL.EscapedPath(), "/")

	return bucket, key, nil
}
