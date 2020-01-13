name = s3fc

GO?=$(shell which go)
SAM?=$(shell which sam)
AWS?=$(shell which aws)
S3FC_INSTANCE_NAME ?= default
s3_stack_name = $(name)-s3-${S3FC_INSTANCE_NAME}
stack_name = $(name)-${S3FC_INSTANCE_NAME}

SOURCE=$(shell find . -name '*.go')

BUCKET=$(shell $(AWS) cloudformation describe-stacks \
	--stack-name ${s3_stack_name} \
	--query 'Stacks[0].Outputs[?OutputKey==`S3FCBucketName`].OutputValue' \
	--output text 2>/dev/null)
S3FCBucketARN=$(shell $(AWS) cloudformation describe-stacks \
	--stack-name ${s3_stack_name} \
	--query 'Stacks[0].Outputs[?OutputKey==`S3FCBucketARN`].OutputValue' \
	--output text 2>/dev/null)
KMS_KEY=$(shell $(AWS) cloudformation describe-stacks \
	--stack-name ${s3_stack_name} \
	--query 'Stacks[0].Outputs[?OutputKey==`S3FCKey`].OutputValue' \
	--output text 2>/dev/null)

.PHONY: all
all: build init install

artifacts:
	mkdir -p artifacts

artifacts/$(name).zip: $(SOURCE) | artifacts
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o artifacts/$(name) $(name)
	cd artifacts && zip $(name).zip $(name)

.PHONY: build
build: artifacts/$(name).zip

.PHONY: test
test:
	 $(GO) test -race -cover ./...

.PHONY: init
init:
	$(AWS) cloudformation deploy \
		--template-file cloudformation/s3_cloudformation.yaml \
		--stack-name $(s3_stack_name) \
		--capabilities CAPABILITY_NAMED_IAM \
		--no-fail-on-empty-changeset \
		--parameter-overrides \
			AppName=$(name) \
			InstanceName=$(S3FC_INSTANCE_NAME)

.PHONY: install
install:
	$(SAM) package \
		--template-file cloudformation/app_cloudformation.yaml \
		--output-template-file cloudformation/app_cloudformation_deploy.yaml \
		--s3-bucket $(BUCKET) \
		--s3-prefix sam-artifacts
	$(SAM) deploy \
		--template-file cloudformation/app_cloudformation_deploy.yaml \
		--stack-name $(stack_name) \
		--capabilities CAPABILITY_NAMED_IAM \
		--no-fail-on-empty-changeset \
		--parameter-overrides \
			AppName=$(name) \
			InstanceName=$(S3FC_INSTANCE_NAME) \
			Bucket=$(S3FCBucketARN) \
			KMSKey=$(KMS_KEY)

.PHONY: uninstall
uninstall:
	scripts/empty_bucket.sh $(AWS) $(BUCKET)
	$(AWS) cloudformation delete-stack --stack-name $(stack_name)
	$(AWS) cloudformation wait stack-delete-complete --stack-name $(stack_name)
	$(AWS) cloudformation delete-stack --stack-name $(s3_stack_name)
	$(AWS) cloudformation wait stack-delete-complete --stack-name $(s3_stack_name)

.PHONY: clean
clean:
	rm -f artifacts/$(name)
	rm -f artifacts/$(name).zip
	rm -f cloudformation_deploy.yaml
