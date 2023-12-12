package main

import (
    "fmt"
    "context"
    "log"
    "os"
    "crypto/tls"
    "crypto/x509"
    "io/ioutil"
    "os/signal"
    "syscall"
    "net/http"
    "encoding/json"
    "strings"

    "github.com/IBM/sarama"
    admissionv1 "k8s.io/api/admission/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

type AdmissionReview struct {
    admissionv1.AdmissionReview
}

//type ResendMessage struct {
//	Body string `json:"body"`
//}

type KafkaMessage struct {
	Kind    string `json:"kind"`
	Version string `json:"apiVersion"`
	Request struct {
		Object struct {
			InvolvedObject struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"involvedObject"`
		} `json:"object"`
	} `json:"request"`
}

func main() {
    // Kafka broker addresses
    brokers := []string{"128.105.145.28:9092"}

    // Create a new consumer
    config := sarama.NewConfig()
    consumer, err := sarama.NewConsumer(brokers, config)
    if err != nil {
 	   fmt.Printf("Error creating Kafka consumer: %v\n", err)
 	   return
    }
    defer consumer.Close()

    // Kafka topic to consume from
    topic := "http-kube"

    // Create a new partition consumer
    partitionConsumer, err := consumer.ConsumePartition(topic, 0, sarama.OffsetOldest)
    if err != nil {
 	   fmt.Printf("Error creating partition consumer: %v\n", err)
 	   return
    }
    defer partitionConsumer.Close()

    // Set up an OS signal channel to gracefully stop the consumer on interrupt or termination
    signals := make(chan os.Signal, 1)
    signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

    // Start a goroutine to handle signals
    go func() {
 	   <-signals
 	   fmt.Println("Received interrupt signal. Shutting down...")
 	   partitionConsumer.AsyncClose()
    }()

    // Start consuming messages
    for {
 	   select {
 	   case msg := <-partitionConsumer.Messages():
 		   // Process the received message
		   //fmt.Printf("Processing message received from Kafka %s\n", msg.Value)
 		   processKafkaMessage(msg.Value)
 	   case err := <-partitionConsumer.Errors():
 		   fmt.Printf("Error consuming message: %v\n", err)
 	   }
    }
}

func processKafkaMessage(message []byte) {
    // Assuming AdmissionReview is a JSON-encoded message
//    var admissionReview AdmissionReview
    //var requestBody []byte
    var kafkaMessage KafkaMessage
    err := json.Unmarshal(message, &kafkaMessage)
    if err != nil {
 	   fmt.Printf("Error decoding JSON: %v\n", err)
 	   return
    }

    // Check if the nested fields exist before accessing them
    var lastAppliedConfigJSON string
    if kafkaMessage.Request.Object != (struct {
		InvolvedObject struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
                        Namespace string `json:"namespace"`
		} `json:"involvedObject"`
	}{}) &&
		kafkaMessage.Request.Object.InvolvedObject != (struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
                        Namespace string `json:"namespace"`
		}{}){
	    // The fields request.object and request.object.involvedObject exist
	    if kafkaMessage.Request.Object.InvolvedObject.Kind != "" {
		// The field request.object.involvedObject.kind exists
		fmt.Println("Kind:", kafkaMessage.Request.Object.InvolvedObject.Kind)
		kubeconfig := "/users/sjain1/.kube/config" // Update with the path to your kubeconfig file
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			fmt.Printf("Error building kubeconfig: %v\n", err)
			return
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Printf("Error creating Kubernetes client: %v\n", err)
			return
		}

		namespace := kafkaMessage.Request.Object.InvolvedObject.Namespace
		resourceName := kafkaMessage.Request.Object.InvolvedObject.Name     // Replace with the actual name of your resource
		resourceType := kafkaMessage.Request.Object.InvolvedObject.Kind  // Replace with the actual type of your resource (Pod, Deployment, StatefulSet)

		lastAppliedConfig, err := getLastAppliedConfigAsJSON(clientset, namespace, resourceName, resourceType)
		if err != nil {
			fmt.Printf("Error getting lastAppliedConfig as JSON: %v\n", err)
			return
		}
		lastAppliedConfigJSON = lastAppliedConfig
	    } else {
		// The field request.object.involvedObject.kind does not exist
		fmt.Println("Kind field does not exist")
		return
	    }
    } else {
	    // The fields request.object or request.object.involvedObject do not exist
	    //fmt.Println("Object or involvedObject fields do not exist")
	    return
    }

    //var resendMessage ResendMessage 
    //err = json.Unmarshal(message, &resendMessage)
    //if err != nil {
     //      fmt.Printf("Error decoding JSON: %v\n", err)
      //     return
   // }

    // Resend the AdmissionReview to the Kubernetes API server
    err = resendToAPIServer(kafkaMessage, lastAppliedConfigJSON)
    if err != nil {
 	   fmt.Printf("Error resending to API server: %v\n", err)
    }

    fmt.Println("Request successfully resent to API server")
}

func getLastAppliedConfigAsJSON(clientset *kubernetes.Clientset, namespace, resourceName, resourceType string) (string, error) {
	var resource interface{}

	switch resourceType {
	case "Pod":
		pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		resource = pod
	case "Deployment":
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		resource = deployment
	case "StatefulSet":
		statefulset, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		resource = statefulset
	default:
		return "", fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	annotations := resource.(metav1.Object).GetAnnotations()
	lastAppliedConfig, found := annotations["kubectl.kubernetes.io/last-applied-configuration"]
	if !found {
		return "", fmt.Errorf("kubectl.kubernetes.io/last-applied-configuration not found in annotations")
	}

	var lastAppliedConfigJSON map[string]interface{}
	err := json.Unmarshal([]byte(lastAppliedConfig), &lastAppliedConfigJSON)
	if err != nil {
		return "", err
	}

	lastAppliedConfigJSONString, err := json.MarshalIndent(lastAppliedConfigJSON, "", "  ")
	if err != nil {
		return "", err
	}

	return string(lastAppliedConfigJSONString), nil
}

func resendToAPIServer(kafkaMsg KafkaMessage, lastConfig string) error {
	// Mark the resource as processed by adding an annotation
//	admissionReview.Request.Object.SetAnnotations(map[string]string{"k8s_processed": "true"})
	//admissionReview = markAsProcessed(admissionReview)
	// Encode the AdmissionReview object back to JSON
	//jsonData, err := json.Marshal(admissionReview)
	log.Printf("Sending API request\n")
	//if err != nil {
	//	return fmt.Errorf("Error encoding AdmissionReview to JSON: %v", err)
	//}

	// Specify the API server URL
	apiServerURL := "https://192.168.49.2:8443/api/v1/namespaces/default/"
	//msgBody := msg.Body
	//msgObj := msgBody.request.object
	//internalObj := nil
	//objKind := nil
	//if msgObj != nil {
	//	internalObj = msgObj.involvedObject
	//} else {
	//	return nil
	//}
	//if internalObj != nil {
	//	objKind = internalObj.kind
	//} else {
	//	return nil
	//}
	apiServerURL = apiServerURL + strings.ToLower(kafkaMsg.Request.Object.InvolvedObject.Kind) + "s"
	fmt.Println("Server url %s", apiServerURL)
	request, err := http.NewRequest(http.MethodPost, apiServerURL, strings.NewReader(lastConfig))
	if err != nil {
		return fmt.Errorf("Error creating HTTP request:", err)
	}
//	apiServerURL := "https://192.168.49.2:8443"
	request.Header.Add("k8s-processed", "true")
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair("/users/sjain1/.minikube/profiles/minikube/client.crt", "/users/sjain1/.minikube/profiles/minikube/client.key")
	if err != nil {
		return fmt.Errorf("Error loading client certificate and key: %v", err)
	}

	// Load CA certificate to verify the server's certificate
	caCert, err := ioutil.ReadFile("/users/sjain1/.minikube/ca.crt")
	if err != nil {
		return fmt.Errorf("Error loading CA certificate: %v", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create a TLS configuration with client certificate, key, and CA certificate
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	// Create an HTTP client with the TLS configuration
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	// Create an HTTP request with the AdmissionReview JSON data
//	req, err := http.NewRequest(http.MethodPost, apiServerURL, bytes.NewBuffer(jsonData))
//	if err != nil {
//		return fmt.Errorf("Error creating HTTP request: %v", err)
//	}

	// Set the Content-Type header
	request.Header.Set("Content-Type", "application/json")

	// Make the HTTP request to the API server
	//client := http.Client{}
	log.Printf("Just sending request to server \n")
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Error making HTTP request: %v", err)
	}
//	body, err := ioutil.ReadAll(resp.Body)
//	if err != nil {
    		// Handle error
//	}
	//log.Printf("API Response Body: %s\n", body)

	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response status: %v", resp.Status)
	}

	return nil
}

//func markAsProcessed(admissionReview AdmissionReview) AdmissionReview {
//	rawExtension := admissionReview.Request.Object

//	if rawExtension.Raw == nil {
//		rawExtension = admissionReview.Request.OldObject
//	}

	// Unmarshal the raw data into an unstructured map
//	objMap := make(map[string]interface{})
//	if err := json.Unmarshal(rawExtension.Raw, &objMap); err != nil {
		// Handle error
//	}

	// Modify the annotations
//	annotations, ok := objMap["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
//	if !ok {
//		annotations = make(map[string]interface{})
//		objMap["metadata"].(map[string]interface{})["annotations"] = annotations
//	}
//	annotations["k8s_processed"] = "true"

	// Encode the modified object back to raw data
//	updatedRaw, err := json.Marshal(objMap)
//	if err != nil {
		// Handle error
//	}
//	log.Printf("Processed request data %s", objMap)

	// Update the RawExtension field
//	admissionReview.Request.Object.Raw = updatedRaw
//	return admissionReview
//}
