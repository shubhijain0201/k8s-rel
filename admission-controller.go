package main

import (
//	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
//	"crypto/tls"
	"os"
//	"strings"
//	"sync"

//	"github.com/confluentinc/confluent-kafka-go/kafka"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/client-go/kubernetes"
//	"k8s.io/client-go/rest"
)

//const (
//	kafkaBroker = "128.105.145.28:9092"
//	kafkaTopic  = "admitrequest"
//	kafkaGroupID = "k8s_admission_consumer_group"
//)

//var (
//	producer     *kafka.Producer
//	producerOnce sync.Once
//)

// Context key for indicating if the request should be intercepted by the admission controller
//type contextKey string

//const (
//	transactionContextKey contextKey = "isTransaction"
//)

var (
	runtimeScheme = runtime.NewScheme()
	codecFactory  = serializer.NewCodecFactory(runtimeScheme)
)

// add kind AdmissionReview in scheme
func init() {
	addToScheme(runtimeScheme)
}

func addToScheme(scheme *runtime.Scheme) {
	corev1.AddToScheme(scheme)
	admissionregistrationv1beta1.AddToScheme(scheme)
}

func main() {
	// Paths to your TLS certificate and private key files
	//certFile := "certificate.pem"
	//keyFile := "private-key.pem"

	// Load the TLS certificate and private key
	//cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	//if err != nil {
	//	log.Fatalf("Error loading TLS certificate and private key: %v", err)
	//}

	log.Printf("Started main\n")

	flag.Parse()
	log.Printf("Parsed the cert info\n")
//	http.HandleFunc("/", admissionHandler)

	// Create a TLS configuration
//	tlsConfig := &tls.Config{
//		Certificates: []tls.Certificate{cert},
//	}

	// Create an HTTP server with TLS
	server := &http.Server{
		Addr:      ":8443", // Choose a suitable port for HTTPS
//		TLSConfig: tlsConfig,
	}
	http.HandleFunc("/", admissionHandler)
	//port := ":8080"
	log.Printf("Server listening on port 8443...\n")
	var err error
	if os.Getenv("SSL_CRT_FILE_NAME") != "" && os.Getenv("SSL_KEY_FILE_NAME") != "" {
		// Starting in HTTPS mode
		err = server.ListenAndServeTLS(os.Getenv("SSL_CRT_FILE_NAME"), os.Getenv("SSL_KEY_FILE_NAME"))
	} else {
		// LOCAL DEV SERVER : Starting in HTTP mode
		err = server.ListenAndServe()
	}
	if err != nil {
		log.Fatal("Server Start Failed : %v", err)
	}
	//log.Fatal(server.ListenAndServeTLS("", ""))
}

func admissionHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Intercepted request \n")
//	if r.Method != http.MethodPost {
//		http.Error(w, "Invalid HTTP method", http.StatusMethodNotAllowed)
//		return
//	}


	// Example: Get the transaction context flag from the request header
//	isTransactionStr := r.Header.Get("X-Is-Transaction")
//	isTransaction := isTransactionStr == "true"

	// Create a context with the transaction flag
//	ctx := context.WithValue(context.Background(), transactionContextKey, isTransaction)

	var admissionReviewReq admissionv1.AdmissionReview
	if err :=  json.NewDecoder(r.Body).Decode(&admissionReviewReq); err != nil {
		http.Error(w, "Failed to decode admission review request", http.StatusBadRequest)
		return
	}
	admissionReviewResponse := handleAdmissionRequest(&admissionReviewReq)

	admissionReview := admissionv1.AdmissionReview{
		Response: admissionReviewResponse,
	}

	if err := json.NewEncoder(w).Encode(admissionReview); err != nil {
		http.Error(w, "Failed to encode admission review response", http.StatusInternalServerError)
		return
	}
}

func handleAdmissionRequest(req *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	log.Printf("Handling intercepted request \n")
	admissionResponse := &admissionv1.AdmissionResponse{
		Allowed: true, // Default to allowing the request
	}

	// Check if the request is a delete operation
	//if req.Request.Operation == admissionv1.Delete {
		// Check if the request should be intercepted based on the transaction context flag
	//	if isTransaction, ok := ctx.Value(transactionContextKey).(bool); ok && isTransaction {
			// Log the delete request to Kafka
			logToKafka(req)

			// Mark the request as processed
			markRequestAsProcessed(req)

			// Deny the request to prevent it from reaching the API server
	//		admissionResponse.Allowed = false
			return admissionResponse
		//}
	//}

	//return admissionResponse
}

//func initializeKafkaProducer() {
	// Create a Kafka producer configuration
//	producerConfig := &kafka.ConfigMap{
//		"bootstrap.servers": kafkaBroker,
//		"client.id":         "k8s_admission_producer",
//	}

	// Create a Kafka producer
//	p, err := kafka.NewProducer(producerConfig)
//	if err != nil {
//		log.Fatalf("Error creating Kafka producer: %v", err)
//	}
//	producer = p
//}

func logToKafka(req *admissionv1.AdmissionReview) {
	log.Printf("trying to log to Kafka \n")
	// Implement Kafka logging logic here
	// Include relevant information from the request in the Kafka log
	// Use a Kafka producer library to send the log to Kafka
	// (Note: The Kafka logging logic is not implemented in this example)
	// Ensure the Kafka producer is initialized only once
//	producerOnce.Do(initializeKafkaProducer)

	// Marshal the AdmissionReview to JSON
	payload, err := json.Marshal(req)
	if err != nil {
		log.Printf("Error marshaling AdmissionReview to JSON: %v", err)
		return
	}
	log.Printf("Payload: %v", payload)
//	topic := kafkaTopic

	// Produce the JSON payload to the Kafka topic
//	err = producer.Produce(&kafka.Message{
//		TopicPartition: kafka.TopicPartition{
//			Topic:     &topic,
//			Partition: kafka.PartitionAny,
//		},
//		Value: payload,
//	}, nil)

//	if err != nil {
//		log.Printf("Error producing message to Kafka: %v", err)
//		return
//	}

	// Wait for message delivery
//	producer.Flush(1000) // 1 second timeout

	// Check for errors during message delivery
//	if err := producer.Flush(15 * 1000); err != nil {
//		log.Printf("Error flushing Kafka producer: %v", err)
//	}
//	failedMessages := producer.Flush(15 * 1000)
//	if failedMessages > 0 {
//		fmt.Printf("%d messages failed to be delivered\n", failedMessages)
//	}
}

func markRequestAsProcessed(req *admissionv1.AdmissionReview) {
	// Unmarshal the raw JSON to a map
	var objMap map[string]interface{}
	if err := json.Unmarshal(req.Request.Object.Raw, &objMap); err != nil {
		fmt.Printf("Error unmarshalling JSON: %v\n", err)
		return
	}

	// Ensure metadata field is not nil
	if objMap["metadata"] == nil {
		objMap["metadata"] = make(map[string]interface{})
	}

	// Ensure annotations field is not nil
	if objMap["metadata"].(map[string]interface{})["annotations"] == nil {
		objMap["metadata"].(map[string]interface{})["annotations"] = make(map[string]interface{})
	}

	// Mark the request as processed by adding an annotation
	objMap["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})["k8s_processed"] = "true"

	// Marshal the modified map back to raw JSON
	modifiedRaw, err := json.Marshal(objMap)
	if err != nil {
		fmt.Printf("Error marshalling modified object: %v\n", err)
		return
	}

	// Update the AdmissionReview's raw object
	req.Request.Object.Raw = modifiedRaw
}

