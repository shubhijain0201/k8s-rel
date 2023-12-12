package injector

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	//"strings"

	"github.com/golang/glog"
	"github.com/IBM/sarama"
	admissionv1 "k8s.io/api/admission/v1"
	//corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
//	"gopkg.in/yaml.v2"
//	"k8s.io/client-go/util/jsonpath"
//	"k8s.io/client-go/util/serializer"
)

//var addInitContainerPatch string

var (
	kafkaProducer     sarama.SyncProducer
	kafkaProducerOnce sync.Once
)

type admitFunc func(admissionv1.AdmissionReview) *admissionv1.AdmissionResponse

// StartServer - Starts Server
func StartServer(port, urlPath string) {
	//addInitContainerPatch = patch
	flag.Parse()

	http.HandleFunc(urlPath, serveMutatePods)
	server := &http.Server{
		Addr: port,
		// Validating cert from client
		// TLSConfig: configTLS(config, getClient()),
	}
	glog.Infof("Starting server at %s", server.Addr)
	var err error
	if os.Getenv("SSL_CRT_FILE_NAME") != "" && os.Getenv("SSL_KEY_FILE_NAME") != "" {
		// Starting in HTTPS mode
		err = server.ListenAndServeTLS(os.Getenv("SSL_CRT_FILE_NAME"), os.Getenv("SSL_KEY_FILE_NAME"))
	} else {
		// LOCAL DEV SERVER : Starting in HTTP mode
		err = server.ListenAndServe()
	}
	if err != nil {
		glog.Errorf("Server Start Failed : %v", err)
	}
}

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	w.Header().Set("Content-Type", "application/json")
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}
	
	url := r.URL
	urlString := url.String()
	glog.Info(fmt.Sprintf("Request URL is %s\n", urlString))
	glog.Info(fmt.Sprintf("Request URL scheme is %s\n", url.Scheme))
	glog.Info(fmt.Sprintf("Request URL host is %s\n", url.Host))
	glog.Info(fmt.Sprintf("Request URL path is %s\n", url.Path))

	// Get query parameters
   	 query := url.Query()

    	// Print all query parameters
    	glog.Info(fmt.Sprintf("Request URL Query Parameters:\n"))
    	for key, values := range query {
        	for _, value := range values {
            		glog.Info(fmt.Sprintf("%s: %s\n", key, value))
        	}
    	}


	// Log the incoming request body to Kafka
    	if err := sendToKafka(body); err != nil {
        	glog.Error(err)
        	response := toAdmissionResponse(err)
		resp, err := json.Marshal(response)
                  if err != nil {
                           glog.Error(err)
                  }
                  glog.V(2).Info(fmt.Sprintf("sending response: %s", string(resp)))
                  if _, err := w.Write(resp); err != nil {
                           glog.Error(err)
                  }
        	// Handle the error response as needed
       // 	sendErrorResponse(w, response)
        	return
    	}

	// Check the "k8s-processed" header
    	if processedHeaderValue := r.Header.Get("k8s-processed"); processedHeaderValue != "" {
        	glog.V(2).Info("Request has already been processed, skipping...")
        	// Respond with success as the request has already been processed
        	response := admissionv1.AdmissionReview{
	            	Response: &admissionv1.AdmissionResponse{
        	        	Allowed: true,
            		},
        	}
		resp, err := json.Marshal(response)
         	if err != nil {
                	 glog.Error(err)
         	}
         	glog.V(2).Info(fmt.Sprintf("sending response: %s", string(resp)))
         	if _, err := w.Write(resp); err != nil {
                	 glog.Error(err)
        	}
        //	sendResponse(w, response)
        	return
    	}



	glog.V(2).Info(fmt.Sprintf("handling request: %s", string(body)))
	var reviewResponse *admissionv1.AdmissionResponse
	ar := admissionv1.AdmissionReview{}
//	arJSON, err := json.MarshalIndent(ar, "", "  ")
//	if err != nil {
//		glog.Error( err)
//		return
//	}
//	glog.Info(fmt.Sprintf("Admission Review content %s", string(arJSON)))
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Error(err)
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}
	//payload, err := json.Marshal(ar)
        //if err != nil {
        //        return
        //}
        //glog.Info(fmt.Sprintf("Can send to Kafka %v", ar))
	response := admissionv1.AdmissionReview{}
	response.APIVersion = "admission.k8s.io/v1"
	response.Kind = "AdmissionReview"
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}
	// reset the Object and OldObject, they are not needed in a response.
	ar.Request.Object = runtime.RawExtension{}
	ar.Request.OldObject = runtime.RawExtension{}

	resp, err := json.Marshal(response)
	if err != nil {
		glog.Error(err)
	}
	glog.V(2).Info(fmt.Sprintf("sending response: %s", string(resp)))
	if _, err := w.Write(resp); err != nil {
		glog.Error(err)
	}
}

func serveMutatePods(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutateResources)
}

func mutateResources(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	glog.V(2).Info("mutating resources")
	requestedResource := metav1.GroupVersionResource{
		Group: ar.Request.Resource.Group, 
		Version: ar.Request.Resource.Version, 
		Resource: ar.Request.Resource.Resource,
	}
	if ar.Request.Resource != requestedResource {
		glog.Errorf("expect resource to be %s", requestedResource)
		return nil
	}

	raw := ar.Request.Object.Raw
	//rawObject := runtime.RawExtension{}
	unknownObject := runtime.Unknown{}
	deserializer := codecs.UniversalDeserializer()
	_, _, err := deserializer.Decode(raw, nil, &unknownObject)
	if err != nil {
		glog.Error(err)
		return toAdmissionResponse(err)
	}
	// Deserialize the raw data into the Unknown object
//	myScheme := runtime.NewScheme()
	// Create a JSON serializer
//	jsonSerializer := runtime.SerializerInfo{
//		Serializer: runtime.ToJSONSerializer(scheme.Scheme),
//		Pretty:     true,
//	}
	//jsonSerializer := serializer.NewCodecFactory(myScheme).UniversalSerializer()
//	obj, gvk, err := runtime.UnstructuredJSONScheme.Decode(unknownObject.Raw, nil, nil)
//	if err != nil {
    		// handle error
//	}

//	jsonData, err := jsonSerializer.Serializer.Encode(obj)
//	if err != nil {
		// handle error
//	}

	// Create an instance of the object's type
//	obj, err := myScheme.New(gvk)
//	if err != nil {
   		// handle error
//	}

	// Convert the Unknown object to JSON
//	jsonData, err := json.Marshal(unknownObject)
//	if err != nil {
    		// handle error
//	}

	// Convert the JSON data back to the original object type
//	err = json.Unmarshal(jsonData, obj)
//	if err != nil {
    		// handle error
//	}

	// Convert the object to YAML

//	decodedObj, _, err := runtime.UnstructuredJSONScheme.Decode(jsonData, nil, nil)
//	if err != nil {
//		// handle error
//	}
	//yamlData, err := yaml.Marshal(unknownObject)
	//if err != nil {
    		// handle error
//	}
	//yamlBytes, err := json.Marshal(obj)
	glog.V(2).Info(fmt.Sprintf("Can send to Kafka %s", unknownObject))
	//objMap := make(map[string]interface{})
        //if err := json.Unmarshal(raw, &objMap); err != nil {
                // Handle error
        //}
	//glog.V(2).Info(fmt.Sprintf("May log the information %v", objMap))

	//if isProcessed(ar) {
	//	glog.V(2).Info("Resource has already been processed, skipping...")
	//	reviewResponse := admissionv1.AdmissionResponse{}
	//	reviewResponse.Allowed = true
	//	return &reviewResponse
	//}

	//payload, err := json.Marshal(ar)
	//if err != nil {
	//	return toAdmissionResponse(err)
	//}
//	glog.V(2).Info(fmt.Sprintf("Can send to Kafka %v", ar.Request))
	//if err := sendToKafka(ar); err != nil {
	//	glog.Error(err)
	//	return toAdmissionResponse(err)
	//}

	// Mark the resource as processed
//	markAsProcessed(&rawObject)

	reviewResponse := admissionv1.AdmissionResponse{}
	reviewResponse.Allowed = true
	//for k, v := range pod.Annotations {
	//	if k == "inject-init-container" && strings.ToLower(v) == "true" {
	//		reviewResponse.Patch = []byte(addInitContainerPatch)
	//		pt := admissionv1.PatchTypeJSONPatch
	//		reviewResponse.PatchType = &pt
	//		return &reviewResponse
	//	}
	//}
	return &reviewResponse
}

func sendToKafka(msg []byte) error {
	// Marshal the AdmissionReview to JSON
	//payload, err := json.Marshal(ar)
	//if err != nil {
	//	return fmt.Errorf("Error marshaling AdmissionReview to JSON: %v", err)
	//}

	// TODO: Replace with your actual Kafka topic
	kafkaTopic := "http-kube"

	// Create a Kafka message with the payload
	message := &sarama.ProducerMessage{
		Topic: kafkaTopic,
		Value: sarama.ByteEncoder(msg),
	}

	// Send the message to Kafka
	_, _, err := getKafkaProducer().SendMessage(message)
	if err != nil {
		return fmt.Errorf("Error sending message to Kafka: %v", err)
	}

	glog.V(2).Info(fmt.Sprintf("Sent HTTP body to Kafka: %s", msg))
	return nil
}

func getKafkaProducer() sarama.SyncProducer {
	kafkaProducerOnce.Do(func() {
		// TODO: Replace with your actual Kafka broker addresses
		kafkaBrokers := []string{"128.105.145.28:9092"}

		// Configure the Kafka producer
		config := sarama.NewConfig()
		config.Producer.Return.Successes = true

		// Create the Kafka producer
		producer, err := sarama.NewSyncProducer(kafkaBrokers, config)
		if err != nil {
			glog.Fatalf("Error creating Kafka producer: %v", err)
		}

		kafkaProducer = producer
		glog.Info("Kafka producer initialized")
	})
	return kafkaProducer
}

//func isProcessed(admissionReview admissionv1.AdmissionReview) bool {
//	annotations := extractAnnotations(admissionReview)
//	glog.Info(fmt.Sprintf("Annotations: %v", annotations))
//	if annotations != nil {
//		return annotations["k8s_processed"] == "true"
//	}
//	return false
//}

//func markAsProcessed(object *runtime.RawExtension) {
	// Ensure metadata field is not nil
//	annotations := extractAnnotations(object)
//	if annotations == nil {
//		annotations = make(map[string]string)
//		setAnnotations(object, annotations)
//	}

	// Mark the resource as processed by adding an annotation
//	annotations["k8s_processed"] = "true"
//}

func extractAnnotations(ar admissionv1.AdmissionReview) (map[string]interface{}) {
	// Type assertion to access the annotations
//	metaObject, ok := object.Object.(metav1.Object)
	metaObject := ar.Request.Object
	//if !ok {
	//	return nil
	//}
	if metaObject.Raw == nil {
		metaObject = ar.Request.OldObject
	}
	objMap := make(map[string]interface{})
        if err := json.Unmarshal(metaObject.Raw, &objMap); err != nil {
                // Handle error
        }
	// Modify the annotations
        annotations, ok := objMap["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
        if !ok {
                annotations = make(map[string]interface{})
                objMap["metadata"].(map[string]interface{})["annotations"] = annotations
        }
	return annotations
}

func setAnnotations(object *runtime.RawExtension, annotations map[string]string) {
	// Type assertion to set the annotations
	metaObject, ok := object.Object.(metav1.Object)
	if ok {
		metaObject.SetAnnotations(annotations)
	}
}

func toAdmissionResponse(err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}
