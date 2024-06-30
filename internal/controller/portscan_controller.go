/*
Copyright 2024 baranitharan.chittharanjan@spark.co.nz.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/barani129/PortScan/api/v1alpha1"
	clusterUtil "github.com/barani129/PortScan/internal/PortScan/util"
)

const (
	defaultHealthCheckInterval = 2 * time.Minute
)

var (
	errGetAuthSecret    = errors.New("failed to get Secret containing External alert system credentials")
	errGetAuthConfigMap = errors.New("failed to get ConfigMap containing the data to be sent to the external alert system")
)

// PortScanReconciler reconciles a PortScan object
type PortScanReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	Kind                     string
	ClusterResourceNamespace string
	recorder                 record.EventRecorder
}

//+kubebuilder:rbac:groups=monitoring.spark.co.nz,resources=portscans,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.spark.co.nz,resources=portscans/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PortScanReconciler) newCluster() (client.Object, error) {
	PortScanGVK := monitoringv1alpha1.GroupVersion.WithKind(r.Kind)
	ro, err := r.Scheme.New(PortScanGVK)
	if err != nil {
		return nil, err
	}
	return ro.(client.Object), nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PortScan object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *PortScanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	cluster, err := r.newCluster()

	if err != nil {
		log.Log.Error(err, "unrecognized Port scan type")
		return ctrl.Result{}, err
	}

	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, fmt.Errorf("unexpected get error : %v", err)
		}
		log.Log.Info("Port scan is not found, ignoring")
		return ctrl.Result{}, nil
	}
	clusterSpec, clusterStatus, err := clusterUtil.GetSpecAndStatus(cluster)
	if err != nil {
		log.Log.Error(err, "unexpected error while getting Port scan spec and status, not trying.")
		return ctrl.Result{}, nil
	}

	secretName := types.NamespacedName{
		Name: clusterSpec.ExternalSecret,
	}

	configmapName := types.NamespacedName{
		Name: clusterSpec.ExternalData,
	}

	switch cluster.(type) {
	case *monitoringv1alpha1.PortScan:
		secretName.Namespace = r.ClusterResourceNamespace
		configmapName.Namespace = r.ClusterResourceNamespace
	default:
		log.Log.Error(fmt.Errorf("unexpected issuer type: %s", cluster), "not retrying")
		return ctrl.Result{}, nil
	}

	var secret corev1.Secret
	var configmap corev1.ConfigMap
	var username []byte
	var password []byte
	var data map[string]string
	if clusterSpec.NotifyExtenal {
		if err := r.Get(ctx, secretName, &secret); err != nil {
			return ctrl.Result{}, fmt.Errorf("%w, secret name: %s, reason: %v", errGetAuthSecret, secretName, err)
		}
		username = secret.Data["username"]
		password = secret.Data["password"]
	}

	if clusterSpec.NotifyExtenal {
		if err := r.Get(ctx, configmapName, &configmap); err != nil {
			return ctrl.Result{}, fmt.Errorf("%w, configmap name: %s, reason: %v", errGetAuthConfigMap, configmapName, err)
		}
		data = configmap.Data
	}

	// report gives feedback by updating the Ready condition of the Port scan
	report := func(conditionStatus monitoringv1alpha1.ConditionStatus, message string, err error) {
		eventType := corev1.EventTypeNormal
		if err != nil {
			log.Log.Error(err, message)
			eventType = corev1.EventTypeWarning
			message = fmt.Sprintf("%s: %v", message, err)
		} else {
			log.Log.Info(message)
		}
		r.recorder.Event(cluster, eventType, monitoringv1alpha1.EventReasonIssuerReconciler, message)
		clusterUtil.SetReadyCondition(clusterStatus, conditionStatus, monitoringv1alpha1.EventReasonIssuerReconciler, message)
	}

	defer func() {
		if err != nil {
			report(monitoringv1alpha1.ConditionFalse, fmt.Sprintf("Trouble reaching the target %s on port %s", clusterSpec.Target, clusterSpec.Port), err)
		}
		if updateErr := r.Status().Update(ctx, cluster); updateErr != nil {
			err = utilerrors.NewAggregate([]error{err, updateErr})
			result = ctrl.Result{}
		}
	}()

	if ready := clusterUtil.GetReadyCondition(clusterStatus); ready == nil {
		report(monitoringv1alpha1.ConditionUnknown, "First Seen", nil)
		return ctrl.Result{}, nil
	}
	filename := fmt.Sprintf("/%s.txt", clusterSpec.Target)
	extFile := fmt.Sprintf("/%s-external.txt", clusterSpec.Target)
	// filename := fmt.Sprintf("/%s.txt", clusterSpec.ClusterFQDN)
	if clusterStatus.LastPollTime == nil {
		log.Log.Info("triggering server FQDN reachability")
		err := clusterUtil.CheckServerAliveness(clusterSpec, clusterStatus)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("Cluster %s is unreachable.", clusterSpec.Target))
			if !clusterSpec.SuspendEmail {
				clusterUtil.SendEmailAlert(filename, clusterSpec)
			}
			if clusterSpec.NotifyExtenal {
				err := clusterUtil.NotifyExternalSystem(data, "firing", clusterSpec.ExternalURL, string(username), string(password), extFile, clusterStatus)
				if err != nil {
					log.Log.Error(err, "Failed to notify the external system")
				}
				fingerprint, err := clusterUtil.ReadFile(extFile)
				fmt.Println(fingerprint)
				if err != nil {
					log.Log.Info("Failed to update the incident ID. Couldn't find the fingerprint in the file")
				}
				incident, err := clusterUtil.SetIncidentID(clusterSpec, clusterStatus, string(username), string(password), fingerprint)
				if err != nil || incident == "" {
					log.Log.Info("Failed to update the incident ID, either incident is getting created or other issues.")
				}
				clusterStatus.IncidentID = incident
			}
			return ctrl.Result{}, fmt.Errorf("%s", err)
		}
		// os.Remove(filename)
		// os.Remove(extFile)
		clusterStatus.ExternalNotified = false
		report(monitoringv1alpha1.ConditionTrue, fmt.Sprintf("Success. Cluster %s is reachable on port %s", clusterSpec.Target, clusterSpec.Port), nil)

	} else {
		pastTime := time.Now().Add(-1 * defaultHealthCheckInterval)
		timeDiff := clusterStatus.LastPollTime.Time.Before(pastTime)
		if timeDiff {
			log.Log.Info("triggering server FQDN reachability as the time elapsed")
			err := clusterUtil.CheckServerAliveness(clusterSpec, clusterStatus)
			if err != nil {
				log.Log.Error(err, fmt.Sprintf("Cluster %s is unreachable.", clusterSpec.Target))
				if !clusterSpec.SuspendEmail {
					clusterUtil.SendEmailAlert(filename, clusterSpec)
				}
				if clusterSpec.NotifyExtenal {
					err := clusterUtil.SubNotifyExternalSystem(data, "firing", clusterSpec.ExternalURL, string(username), string(password), extFile, clusterStatus)
					if err != nil {
						log.Log.Error(err, "Failed to notify the external system")
					}
					fingerprint, err := clusterUtil.ReadFile(extFile)
					if err != nil {
						log.Log.Info("Failed to update the incident ID. Couldn't find the fingerprint in the file")
					}
					incident, err := clusterUtil.SetIncidentID(clusterSpec, clusterStatus, string(username), string(password), fingerprint)
					if err != nil || incident == "" {
						log.Log.Info("Failed to update the incident ID, either incident is getting created or other issues.")
					}
					clusterStatus.IncidentID = incident
				}
				return ctrl.Result{}, fmt.Errorf("%s", err)
			}

			if _, err := os.Stat(extFile); os.IsNotExist(err) {
				// no action
			} else {
				if !clusterSpec.SuspendEmail {
					clusterUtil.SendEmailReachableAlert(filename, clusterSpec)
				}
				if clusterSpec.NotifyExtenal {
					err := clusterUtil.SubNotifyExternalSystem(data, "resolved", clusterSpec.ExternalURL, string(username), string(password), extFile, clusterStatus)
					if err != nil {
						log.Log.Error(err, "Failed to notify the external system")
					}
					now := metav1.Now()
					clusterStatus.ExternalNotifiedTime = &now
				}
				os.Remove(filename)
				os.Remove(extFile)
			}
			clusterStatus.ExternalNotified = false
			report(monitoringv1alpha1.ConditionTrue, fmt.Sprintf("Success. Cluster %s is reachable on port %s", clusterSpec.Target, clusterSpec.Port), nil)
		}
	}
	return ctrl.Result{RequeueAfter: defaultHealthCheckInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PortScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor(monitoringv1alpha1.EventSource)
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.PortScan{}).
		Complete(r)
}
