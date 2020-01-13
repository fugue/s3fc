name=example-s3fc-job

AWS=$(shell which aws)


S3FC_EXTERNAL_ID ?= example-external-id

BUCKET=$(shell $(AWS) cloudformation describe-stacks \
	--stack-name ${name} \
	--query 'Stacks[0].Outputs[?OutputKey==`ExampleBucketName`].OutputValue' \
	--output text 2>/dev/null)


.PHONY: all
ifndef S3FC_ROLE_ARN
all:
	$(error S3FC_ROLE_ARN is not set)
else
all:
	$(AWS) cloudformation deploy \
		--template-file cloudformation.yaml \
		--stack-name $(name) \
		--capabilities CAPABILITY_NAMED_IAM \
		--no-fail-on-empty-changeset \
		--parameter-overrides \
			S3FCRoleArn=$(S3FC_ROLE_ARN) \
			ExternalId=$(S3FC_EXTERNAL_ID)
endif

.PHONY: upload_data
upload_data:
	$(AWS) s3 cp --recursive example-source-data/ s3://${BUCKET}/example-source-data
