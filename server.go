package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"

	// "github.com/golang/glog"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	// "github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2016-10-01/keyvault"
	kv "github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	// "github.com/Azure/go-autorest/autorest"

	k8spb "github.com/ritazh/k8s-azure-kms/v1beta1"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"os"
)

const (
	// Unix Domain Socket
	netProtocol    = "unix"
	socketPath	   = "/tmp/azurekms.socket"
	version        = "v1beta1"
	runtime        = "Microsoft AzureKMS"
	runtimeVersion = "0.0.1"
	configFilePath  = "/etc/kubernetes/azure.json"
)

type KMSServiceServer struct {
	pathToUnixSocket string
	net.Listener
	*grpc.Server
}

func New(pathToUnixSocketFile string) *KMSServiceServer {
	kmsServiceServer := new(KMSServiceServer)
	kmsServiceServer.pathToUnixSocket = pathToUnixSocketFile
	fmt.Println(kmsServiceServer.pathToUnixSocket)
	return kmsServiceServer
}

func getKeysClient() (kv.ManagementClient, string, string, string, error)  {
	vmClient := kv.New()
	token, err := GetKeyvaultToken(AuthGrantType(), configFilePath)
	if err != nil {
		return vmClient, "", "", "", fmt.Errorf("failed to get token, error: %v", err)
	}
	
	vmClient.Authorizer = token
	vaultBaseURL, keyName, keyVersion, err := GetKMSProvider(configFilePath)
	if err != nil {
		return vmClient, "", "", "", fmt.Errorf("failed to get KMS provider configs, error: %v", err)
	}
	return vmClient, *vaultBaseURL, *keyName, *keyVersion, nil
}

// doEncrypt encrypts with an existing key
func doEncrypt(ctx context.Context, data []byte) (*string, error) {
	vaultsClient, vaultBaseURL, keyName, keyVersion, err := getKeysClient()
	if err != nil {
		return nil, err
	}

 	value := base64.RawURLEncoding.EncodeToString(data)
	parameter := kv.KeyOperationsParameters {
		Algorithm: kv.RSA15,
		Value: &value,
	}
	
	result, err := vaultsClient.Encrypt(vaultBaseURL, keyName, keyVersion, parameter)
	if err != nil {
		fmt.Print("failed to encrypt, error: %v", err)
		return nil, err
	}
	return result.Result, nil
}

// doDecrypt decrypts with an existing key
func doDecrypt(ctx context.Context, data string) ([]byte, error) {
	vaultsClient, vaultBaseURL, keyName, keyVersion, err := getKeysClient()
	if err != nil {
		return nil, err
	}
	parameter := kv.KeyOperationsParameters {
		Algorithm: kv.RSA15,
		Value: &data,
	}
	
	result, err := vaultsClient.Decrypt(vaultBaseURL, keyName, keyVersion, parameter)
	if err != nil {
		fmt.Print("failed to decrypt, error: %v", err)
		return nil, err
	}
	bytes, err := base64.RawURLEncoding.DecodeString(*result.Result)
	return bytes, nil
}

func main() {

	var (
		debugListenAddr = flag.String("debug-listen-addr", "127.0.0.1:7901", "HTTP listen address.")
	)
	flag.Parse()

	log.Println("KMSServiceServer service starting...")
	s := New(socketPath)
	if err := s.cleanSockFile(); err != nil {
		fmt.Errorf("failed to clean sockfile, error: %v", err)
	}

	listener, err := net.Listen(netProtocol, s.pathToUnixSocket)
	if err != nil {
		fmt.Errorf("failed to start listener, error: %v", err)
	}
	s.Listener = listener

	server := grpc.NewServer()
	k8spb.RegisterKMSServiceServer(server, s)
	s.Server = server

	go server.Serve(listener)

	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) { return true, true }
	log.Println("KMSServiceServer service started successfully.")
	log.Fatal(http.ListenAndServe(*debugListenAddr, nil))
}

func (s *KMSServiceServer) Version(ctx context.Context, request *k8spb.VersionRequest) (*k8spb.VersionResponse, error) {
	fmt.Println("Processing Version: ")
	fmt.Println(version)
	return &k8spb.VersionResponse{Version: version, RuntimeName: runtime, RuntimeVersion: runtimeVersion}, nil
}

func (s *KMSServiceServer) Encrypt(ctx context.Context, request *k8spb.EncryptRequest) (*k8spb.EncryptResponse, error) {

	fmt.Println("Processing EncryptRequest: ")
	cipher, err := doEncrypt(ctx, request.Plain)
	if err != nil {
		fmt.Print("failed to doencrypt, error: ", err)
		return &k8spb.EncryptResponse{}, err
	}
	return &k8spb.EncryptResponse{Cipher: []byte(*cipher)}, nil
}

func (s *KMSServiceServer) Decrypt(ctx context.Context, request *k8spb.DecryptRequest) (*k8spb.DecryptResponse, error) {

	fmt.Println("Processing DecryptRequest: ")
	plain, err := doDecrypt(ctx, string(request.Cipher))
	if err != nil {
		fmt.Print("failed to decrypt, error: ", err)
		return &k8spb.DecryptResponse{}, err
	}
	return &k8spb.DecryptResponse{Plain: plain}, nil
}

func (s *KMSServiceServer) cleanSockFile() error {
	err := unix.Unlink(s.pathToUnixSocket)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete the socket file, error: %v", err)
	}
	return nil
}
