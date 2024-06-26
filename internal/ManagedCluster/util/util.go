package util

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/barani129/MgtCluster/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSpecAndStatus(MgtCluster client.Object) (*v1alpha1.MgtClusterSpec, *v1alpha1.MgtClusterStatus, error) {
	switch t := MgtCluster.(type) {
	case *v1alpha1.MgtCluster:
		return &t.Spec, &t.Status, nil
	default:
		return nil, nil, fmt.Errorf("not a managed cluster type: %t", t)
	}
}

func GetReadyCondition(status *v1alpha1.MgtClusterStatus) *v1alpha1.MgtClusterCondition {
	for _, c := range status.Conditions {
		if c.Type == v1alpha1.MgtClusterConditionReady {
			return &c
		}
	}
	return nil
}

func IsReady(status *v1alpha1.MgtClusterStatus) bool {
	if c := GetReadyCondition(status); c != nil {
		return c.Status == v1alpha1.ConditionTrue
	}
	return false
}

func SetReadyCondition(status *v1alpha1.MgtClusterStatus, conditionStatus v1alpha1.ConditionStatus, reason, message string) {
	ready := GetReadyCondition(status)
	if ready == nil {
		ready = &v1alpha1.MgtClusterCondition{
			Type: v1alpha1.MgtClusterConditionReady,
		}
		status.Conditions = append(status.Conditions, *ready)
	}
	if ready.Status != conditionStatus {
		ready.Status = conditionStatus
		now := metav1.Now()
		ready.LastTransitionTime = &now
	}
	ready.Reason = reason
	ready.Message = message
	for i, c := range status.Conditions {
		if c.Type == v1alpha1.MgtClusterConditionReady {
			status.Conditions[i] = *ready
			return
		}
	}
}

func CheckServerAliveness(spec *v1alpha1.MgtClusterSpec, status *v1alpha1.MgtClusterStatus) error {
	// url := fmt.Sprintf(`https://%s:%s`, spec.ClusterFQDN, spec.Port)
	// tr := http.Transport{
	// 	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// }
	// client := &http.Client{
	// 	Timeout:   5 * time.Second,
	// 	Transport: &tr,
	// }

	// req, err := http.NewRequest("GET", url, nil)
	// if err != nil {
	// 	return err
	// }
	// resp, err := client.Do(req)
	// if err != nil {
	// 	return err
	// }
	// if resp.StatusCode != 200 || resp == nil {
	// 	return fmt.Errorf("cluster %s is unreachable", spec.ClusterFQDN)
	// }
	command := fmt.Sprintf("/usr/bin/nc -w 3 -zv %s %s", spec.ClusterFQDN, spec.Port)
	cmd := exec.Command("/bin/bash", "-c", command)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("cluster %s is unreachable", spec.ClusterFQDN)
	}
	now := metav1.Now()
	status.LastPollTime = &now
	return nil
}

func randomString(length int) string {
	b := make([]byte, length+2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[2 : length+2]
}

func SubNotifyExternalSystem(data map[string]string, status string, url string, username string, password string, filename string, clstatus *v1alpha1.MgtClusterStatus) error {
	var fingerprint string
	var err error
	if status == "resolved" {
		fingerprint, err = ReadFile(filename)
		if err != nil || fingerprint == "" {
			return fmt.Errorf("unable to notify the system for the %s status due to missing fingerprint in the file %s", status, filename)
		}
	} else {
		fingerprint, _ = ReadFile(filename)
		if fingerprint != "" {
			return nil
		}
		fingerprint = randomString(10)
	}
	data["fingerprint"] = fingerprint
	data["status"] = status
	data["startsAt"] = time.Now().String()
	m, b := data, new(bytes.Buffer)
	json.NewEncoder(b).Encode(m)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Basic "+basicAuth(username, password))
	req.Header.Set("User-Agent", "Openshift")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp == nil {
		return err
	}
	writeFile(filename, fingerprint)
	clstatus.ExternalNotified = true
	now := metav1.Now()
	clstatus.ExternalNotifiedTime = &now
	return nil
}

func NotifyExternalSystem(data map[string]string, status string, url string, username string, password string, filename string, clstatus *v1alpha1.MgtClusterStatus) error {
	fingerprint := randomString(10)
	data["fingerprint"] = fingerprint
	data["status"] = status
	data["startsAt"] = time.Now().String()
	m, b := data, new(bytes.Buffer)
	json.NewEncoder(b).Encode(m)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Basic "+basicAuth(username, password))
	req.Header.Set("User-Agent", "Openshift")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp == nil {
		return err
	}
	writeFile(filename, fingerprint)
	clstatus.ExternalNotified = true
	return nil
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func SendEmailAlert(filename string, spec *v1alpha1.MgtClusterSpec) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		FQDN := spec.ClusterFQDN
		if FQDN != "" {
			message := fmt.Sprintf(`/bin/echo "cluster %s is unreachable" | /usr/sbin/sendmail -f %s -S %s %s`, FQDN, spec.Email, spec.RelayHost, spec.Email)
			cmd3 := exec.Command("/bin/bash", "-c", message)
			err := cmd3.Run()
			if err != nil {
				fmt.Printf("Failed to send the alert: %s", err)
			}
			writeFile(filename, "sent")
		}
	} else {
		data, _ := ReadFile(filename)
		fmt.Println(data)
		if data != "sent" {
			FQDN := spec.ClusterFQDN
			if FQDN != "" {
				message := fmt.Sprintf(`/bin/echo "cluster %s is unreachable" | /usr/sbin/sendmail -f %s -S %s %s`, FQDN, spec.Email, spec.RelayHost, spec.Email)
				cmd3 := exec.Command("/bin/bash", "-c", message)
				err := cmd3.Run()
				if err != nil {
					fmt.Printf("Failed to send the alert: %s", err)
				}
			}
		}

	}
}

func SendEmailReachableAlert(filename string, spec *v1alpha1.MgtClusterSpec) {
	data, err := ReadFile(filename)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(data)
	if data == "sent" {
		FQDN := spec.ClusterFQDN
		if FQDN != "" {
			message := fmt.Sprintf(`/bin/echo "cluster %s is reachable again" | /usr/sbin/sendmail -f %s -S %s %s`, FQDN, spec.Email, spec.RelayHost, spec.Email)
			cmd3 := exec.Command("/bin/bash", "-c", message)
			err := cmd3.Run()
			if err != nil {
				fmt.Printf("Failed to send the alert: %s", err)
			}
		}
	}
}

func writeFile(filename string, data string) error {
	err := os.WriteFile(filename, []byte(data), 0644)
	if err != nil {
		return err
	}
	return nil
}

func ReadFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
