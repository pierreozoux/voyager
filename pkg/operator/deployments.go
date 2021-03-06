package operator

import (
	"github.com/appscode/go/types"
	"github.com/appscode/log"
	"github.com/appscode/voyager/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// Blocks caller. Intended to be called as a Go routine.
func (op *Operator) initDeploymentWatcher() cache.Controller {
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return op.KubeClient.ExtensionsV1beta1().Deployments(apiv1.NamespaceAll).List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return op.KubeClient.ExtensionsV1beta1().Deployments(apiv1.NamespaceAll).Watch(metav1.ListOptions{})
		},
	}
	_, informer := cache.NewInformer(lw,
		&extensions.Deployment{},
		op.Opt.ResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj interface{}) {
				if deployment, ok := obj.(*extensions.Deployment); ok {
					log.Infof("Deployment %s@%s deleted", deployment.Name, deployment.Namespace)
					op.restoreDeploymentIfRequired(deployment)
				}
			},
		},
	)
	return informer
}

func (op *Operator) restoreDeploymentIfRequired(deployment *extensions.Deployment) error {
	if deployment.Annotations == nil {
		return nil
	}

	// deleted resource have source reference
	engress, err := op.findOrigin(deployment.ObjectMeta)
	if err != nil {
		return err
	}

	// Ingress Still exists, restore resource
	log.Infof("Deployment %s@%s requires restoration", deployment.Name, deployment.Namespace)
	deployment.Spec.Paused = false
	if types.Int32(deployment.Spec.Replicas) < 1 {
		deployment.Spec.Replicas = types.Int32P(engress.Replicas())
	}
	deployment.SelfLink = ""
	deployment.ResourceVersion = ""
	// Old resource and annotations are missing so we need to add the annotations
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[api.OriginAPISchema] = engress.APISchema()
	deployment.Annotations[api.OriginName] = engress.Name

	_, err = op.KubeClient.ExtensionsV1beta1().Deployments(deployment.Namespace).Create(deployment)
	return err
}
