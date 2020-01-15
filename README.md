# S3FC

This application takes sets of uncompressed text files in the same s3 bucket prefix and concatenates them into larger files as per [job configuration](#job-input). The initial use case for this application is to prepare a data set of many (millions+) small files of immutable data for batch processing or incremental processing. It is built on top of serverless platforms offered by AWS, Lambda and Step Functions.

## Table of contents
* [Build and Deploy Dependencies](#build-and-deploy-dependencies)
* [Quick Build and Deployment](#quick-build-and-deployment)
* [Make Targets](#make-targets)
* [Deploying Example and Running Job](#deploying-example-and-running-job)
* [Definitions](#definitions)
* [Job Input](#job-input)

## Build and Deploy Dependencies

### Amazon Web Services Account
* Create an AWS account: https://aws.amazon.com/
* Create programmatic access: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_users_create.html#id_users_create_api 

### go
* Install via: [https://golang.org/dl/](https://golang.org/dl/)
* Docs: [https://golang.org/doc/](https://golang.org/doc/)
* Source: [https://go.googlesource.com/go](https://go.googlesource.com/go)

### awscli
* Install via: `pip install awscli`
* Docs: [https://aws.amazon.com/cli/](https://aws.amazon.com/cli/)
* Source: [https://github.com/aws/aws-cli](https://github.com/aws/aws-cli)

### aws sam cli
* Install via: `pip install aws-sam-cli`
* Docs: [https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-command-reference.html](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-command-reference.html)
* Source: [https://github.com/awslabs/aws-sam-cli](https://github.com/awslabs/aws-sam-cli)

### make
* Install via: ... apt-get install, emerge make, brew install, windows?, etc... it's make.
* Docs: [https://www.gnu.org/software/make/manual/make.html](https://www.gnu.org/software/make/manual/make.html)
* Source: [http://git.savannah.gnu.org/cgit/make.git](http://git.savannah.gnu.org/cgit/make.git)

## Quick Build and Deployment

If you have only just installed the aws cli and already created credentials for programmatic access
```
aws configure
```

Full build and deploy
```
make
```

## Make Targets

### test
`make test`

Runs unit tests.

### init
`make init S3FC_INSTANCE_NAME=default`

Installs a KMS Key and S3 bucket into the environment's configured AWS account. This bucket is used to host lambda artifacts and S3FC state/database files.

Variables
* S3FC_INSTANCE_NAME: (optional, default=default) Set to something unique if you want to have multiple S3FC instances deployed to the same account.

### build
`make build`

Builds project artifacts targeting amd64/linux systems. Writes them to `./artifacts`

### install
`make install S3FC_INSTANCE_NAME=default`

Deploys artifacts to the enviroment's configured AWS account.

Variables
* S3FC_INSTANCE_NAME: (optional, default=default) Set to something unique if you want to have multiple S3FC instances deployed to the same account.

### uninstall
`make uninstall S3FC_INSTANCE_NAME=default`

Empties installed buckets and uninstalls all S3FC managed stacks.

Variables
* S3FC_INSTANCE_NAME: (optional, default=default) Set to something unique if you want to have multiple S3FC instances deployed to the same account.

## Deploying Example and Running Job

In order to keep S3FC's default IAM permissions scoped tightly and to allow for potential cross account file management, each "job" will assume a provided IAM role that tailors its permissions to a specific set of source and destination S3 objects.

The example job CloudFormation file will create another S3 bucket and KMS key for the job's source files and destination files. It will also create a role that S3FC will assume that grants granular access to the resources it will be managing.

1. Deploy the example resources, granting the S3FC Role access to assume the newly created role. This will also upload an extremely small dataset for smoke testing the installation.
```
S3FC_INSTANCE_NAME=default
S3FC_EXTERNAL_ID=example-external-id
S3FC_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name s3fc-${S3FC_INSTANCE_NAME} \
    --query 'Stacks[0].Outputs[?OutputKey==`S3FCFunctionRole`].OutputValue' \
	--output text 2>/dev/null)
make -C examples S3FC_ROLE_ARN=${S3FC_ROLE_ARN} S3FC_EXTERNAL_ID=${S3FC_EXTERNAL_ID}
make -C examples upload_data
```

2. Visit the Step Functions AWS Console (https://console.aws.amazon.com/states/home).
3. Click into the S3FC (s3fc-default) state machine. If you've already successfully deployed the application and don't see it, double check that you are looking at the correct region.
4. Click the "Start Execution" button.
5. Copy the contents of `examples/statemachine-input.json` into the input box.
6. Replace the XXXXXXXXXXXX's with the AWS account ID the example is deployed to. Probably found by doing `aws sts get-caller-identity`
7. Replace the YYYYYYYYY's with the region the example job has been deployed to.
8. Click the "Start execution" in the lower right of the modal.
9. The step function will start a new execution and should complete successfully within a few seconds.
10. Visit the S3 AWS Console (https://s3.console.aws.amazon.com/s3/home).
11. Click in the bucket named like `example-job-<region>-<account_id>` 
12. You should see a directory named `example-destination-data` with a single file containing the concatenated data from `example-source-data`


Cleaning up the example:
```
make -C examples clean
```

## Definitions

Term | Definition
---|---
Client | The entity that hosts the source and destination files.
Driver | An external application that manages client job configuration and triggers executions. This application is external to this project.
Service | The S3FC lambda and state machine. Simply invoked by a "driver" by calling [StepFunctions.StartExecution](https://docs.aws.amazon.com/step-functions/latest/apireference/API_StartExecution.html)


## Job Input

Example:
```
{
    "input": {
        "assume_role": "arn:aws:iam::XXXXXXXXXXXX:role/example-job-role",
        "external_id": "example-external-id",
        "bolt_db_url": "s3://s3fc-YYYYYYYYY-XXXXXXXXXXXX-default/s3fc/example_job.bdb",
        "inventory_url": "s3://s3fc-YYYYYYYYY-XXXXXXXXXXXX-default/s3fc/example_job.json",
        "bucket": "example-job-YYYYYYYYY-XXXXXXXXXXXX",
        "prefix": "example-source-data",
        "destination_bucket": "example-job-YYYYYYYYY-XXXXXXXXXXXX",
        "destination_path": "example-destination-data",
        "block_size": 1048576,
        "delimiter": ""
    }
}
```

Property Name | Type | Description
---|:---:|---
assume_role | `string` | **Required.** The role S3FC will assume to operate on source and destination files. This is supplied by the "client."
external_id | `string` | **Required.** The External ID that S3FC will supply the call to AssumeRole. It is supplied by the "driver" and configured on the "client." This value should be unique and a secret between the driver and client. This prevents other clients from using other clients' non-secret Role ARN. For more on this topic see [documentation here](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-user_externalid.html). 
bolt_db_url | `string` | **Required.** The location that the job's boltdb database will be stored to and loaded from. This is provided by the "driver."
inventory_url | `string` | **Required.** The location that the job's source file inventory will be stored to and loaded from. This is provided by the "driver."
bucket | `string` | **Required.** The bucket that contains the source files.
prefix | `string` | **Required.** An object key prefix that will list the targeted source files.
destination_bucket | `string` | **Required.** The bucket where the destination files will be written to.
destination_path | `string` | **Required.** The path where the destination files will be written to.
block_size | `integer` | **Required.** The targeted size of destination files in bytes.
delimiter | `string` | **Required (empty value allowed).** A string that acts as a delimiter between source files inside of a destination file. An example being, if your source files are not new line terminated you may want to set this value to `"\n"` so that records are on individual lines in the file. If your files are new line terminated and you want source files to delimited by new lines, you could set this value to an empty string `""`
