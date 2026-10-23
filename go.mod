module github.com/gilbertchen/duplicacy

go 1.26.0

toolchain go1.26.10

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.23.3
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.8.0
	github.com/aryann/difflib v0.0.0-20170710044230-e206f873d14a
	github.com/aws/aws-sdk-go-v2 v1.47.4
	github.com/aws/aws-sdk-go-v2/credentials v1.20.10
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.5
	github.com/bkaradzic/go-lz4 v1.0.0
	github.com/cloudsoda/go-smb2 v0.0.0-20260918041005-0c5d69b69701
	github.com/fsouza/fake-gcs-server v1.56.1
	github.com/gilbertchen/cli v1.2.1-0.20160223210219-1de0a1836ce9
	github.com/gilbertchen/go-dropbox v0.0.0-20230321030224-087ef8db1916
	github.com/gilbertchen/go-ole v1.2.0
	github.com/gilbertchen/goamz v0.0.0-20170712012135-eada9f4e8cc2
	github.com/gilbertchen/gopass v0.0.0-20170109162249-bf9dde6d0d2c
	github.com/gilbertchen/keyring v0.0.0-20221004152639-1661cbebc508
	github.com/gilbertchen/xattr v0.0.0-20160926155429-68e7a6806b01
	github.com/johannesboyne/gofakes3 v1.2.0
	github.com/klauspost/compress v1.20.3
	github.com/klauspost/reedsolomon v1.9.9
	github.com/minio/highwayhash v1.0.2
	github.com/ncw/swift/v2 v2.0.1
	github.com/pierrec/lz4/v4 v4.1.28
	github.com/pkg/sftp v1.11.0
	github.com/pkg/xattr v0.4.12
	github.com/vmihailenco/msgpack/v5 v5.4.1
	golang.org/x/crypto v0.58.0
	golang.org/x/oauth2 v0.37.0
	golang.org/x/term v0.47.0
	google.golang.org/api v0.308.0
	storj.io/uplink v1.12.1
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.24.1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.3.0 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	cloud.google.com/go/iam v1.12.0 // indirect
	cloud.google.com/go/monitoring v1.30.0 // indirect
	cloud.google.com/go/pubsub/v2 v2.6.2 // indirect
	cloud.google.com/go/storage v1.65.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.13.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.34.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.57.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.57.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.24 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.7 // indirect
	github.com/aws/smithy-go v1.28.6 // indirect
	github.com/calebcase/tmpfile v1.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudsoda/sddl v0.0.0-20250224235906-926454e91efc // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/flynn/noise v1.0.0 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goamz/goamz v0.0.0-20180131231218-8b901b531db8 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/renameio/v2 v2.0.0 // indirect
	github.com/google/s2a-go v0.1.11 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.23 // indirect
	github.com/googleapis/gax-go/v2 v2.26.2 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jtolio/eventkit v0.0.0-20221004135224-074cf276595b // indirect
	github.com/jtolio/noiseconn v0.0.0-20230111204749-d7ec1a08b0b8 // indirect
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mmcloughlin/avo v0.0.0-20200803215136-443f81d77104 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/ryszard/goskiplist v0.0.0-20150312221310-2dfbae5fcf46 // indirect
	github.com/segmentio/go-env v1.1.0 // indirect
	github.com/spacemonkeygo/monkit/v3 v3.0.22 // indirect
	github.com/spiffe/go-spiffe/v2 v2.8.1 // indirect
	github.com/vaughan0/go-ini v0.0.0-20130923145212-a98ad7ee00ec // indirect
	github.com/vivint/infectious v0.0.0-20200605153912-25a574ae18a3 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/zeebo/blake3 v0.2.3 // indirect
	github.com/zeebo/errs v1.4.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.44.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	go.shabbyrobe.org/gocovmerge v0.0.0-20230507111327-fa4f82cfbf4d // indirect
	golang.org/x/exp v0.0.0-20260813180055-c1d0aacb2297 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	google.golang.org/genproto v0.0.0-20260715232425-e75dac1f907d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260715232425-e75dac1f907d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	storj.io/common v0.0.0-20230920095429-0ce0a575e6f8 // indirect
	storj.io/drpc v0.0.33 // indirect
	storj.io/picobuf v0.0.2-0.20230906122608-c4ba17033c6c // indirect
)
