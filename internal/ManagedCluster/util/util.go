package util

import (
	"fmt"
	"os"
	"os/exec"

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
	cmd := exec.Command("/bin/bash", "/usr/bin/nc", "-zv", spec.ClusterFQDN, spec.Port)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("cluster %s is unreachable", spec.ClusterFQDN)
	}
	now := metav1.Now()
	status.LastPollTime = &now
	return nil
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
			writeFile(filename, spec)
		}
	} else {
		data, _ := ReadFile(filename, spec)
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

func writeFile(filename string, spec *v1alpha1.MgtClusterSpec) error {
	err := os.WriteFile(filename, []byte("sent"), 0644)
	if err != nil {
		return err
	}
	return nil
}

func ReadFile(filename string, spec *v1alpha1.MgtClusterSpec) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
