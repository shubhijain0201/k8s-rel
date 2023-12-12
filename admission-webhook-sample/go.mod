module github.com/dinumathai/admission-webhook-sample

go 1.16

replace admission-webhook-sample/injector => ./injector

require (
	github.com/IBM/sarama v1.42.1
	github.com/golang/glog v1.0.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.28.4
	k8s.io/apimachinery v0.28.4
	k8s.io/client-go v0.28.4
)
