/*
Copyright 2022.

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

package controllers

import (
	"context"
	"encoding/json"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appzhangsichencnv1 "github.com/ZSCREDBACK/operator-demo/api/v1"
)

// AppServiceReconciler reconciles a AppService object
type AppServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=app.zhangsichen.cn.zhangsichen.cn,resources=appservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app.zhangsichen.cn.zhangsichen.cn,resources=appservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=app.zhangsichen.cn.zhangsichen.cn,resources=appservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *AppServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// 记录发生了Reconcile操作的对象日志
	reqLogger := log.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling AppService")

	// Fetch the AppService instance
	instance := &appzhangsichencnv1.AppService{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		// 如果是找不到,说明这个cr已经被删除了
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			// 返回资源不存在的错误日志
			reqLogger.Info("AppService resource not found. Ignoring since object must be deleted")
			// 停止loop循环,不再订阅事件
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		// 返回错误,但是继续监听事件
		return ctrl.Result{}, err
	}

	// 思路:
	// 1.如果资源不存在,则创建关联资源
	// 2.如果资源存在,判断是否需要更新
	// 3.如果资源需要更新,则直接更新
	// 4.如果资源不需要更新,则正常返回

	// TODO(user): your logic here

	// 确认cr下的deploy是否存在
	deploy := &appsv1.Deployment{}

	// 获取当前NS下的Deployment资源,并判断是否存在
	// 如果资源不存在,则创建相关资源
	if err := r.Client.Get(ctx, req.NamespacedName, deploy); err != nil && errors.IsNotFound(err) {
		// 1. 创建Deployment资源
		deploy := r.deploymentForAppService(instance) // 将cr转换为deployment资源(单独封装)
		if err := r.Client.Create(ctx, deploy); err != nil {
			reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", deploy.Namespace, "Deployment.Name", deploy.Name)
			return ctrl.Result{}, err
		}

		// 2. 创建Service资源
		service := r.serviceForAppService(instance) // 将cr转换为service资源(单独封装)
		if err := r.Client.Create(ctx, service); err != nil {
			reqLogger.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}

		// 3. 添加Annotation信息
		data, _ := json.Marshal(instance.Spec)
		if instance.Annotations != nil {
			instance.Annotations["spec"] = string(data)
		} else {
			instance.Annotations = map[string]string{"spec": string(data)}
		}

		// 4. 更新AppService资源
		if err := r.Client.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// 反之,如果资源存在,则判断是否需要更新
	// 先获取旧的spec
	oldSpec := &appzhangsichencnv1.AppServiceSpec{}
	if err := json.Unmarshal([]byte(instance.Annotations["spec"]), oldSpec); err != nil {
		return ctrl.Result{}, err
	}

	// 判断是否需要更新
	if !reflect.DeepEqual(instance.Spec, *oldSpec) { // 判断两个结构体是否相等
		// 如果不相等,则更新相关资源
		newDeploy := r.deploymentForAppService(instance)
		oldDeploy := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, req.NamespacedName, oldDeploy); err != nil {
			return ctrl.Result{}, err
		}
		oldDeploy.Spec = newDeploy.Spec
		if err := r.Client.Update(ctx, oldDeploy); err != nil {
			return ctrl.Result{}, err
		}

		newService := r.serviceForAppService(instance) // 将cr转换为service资源
		oldService := &corev1.Service{}
		if err := r.Client.Get(ctx, req.NamespacedName, oldService); err != nil {
			return ctrl.Result{}, err
		}
		oldService.Spec = newService.Spec
		if err := r.Client.Update(ctx, oldService); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// for表示监控的cr的类型,Owns表示监控的第二资源也就是cr需要控制的资源
func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// watch AppService这个crd作为第一监控资源
		For(&appzhangsichencnv1.AppService{}).
		// watch Deployment这个资源作为第二监控资源
		Owns(&appsv1.Deployment{}). // 增加了此行
		Complete(r)                 // 传入Reconciler
}

// 根据 CRD 中的声明去填充 Deployment 的内容
// deploymentForAppService returns a busybox pod with the same name/namespace as the CR
func (r *AppServiceReconciler) deploymentForAppService(instance *appzhangsichencnv1.AppService) *appsv1.Deployment {
	// 定义一个Deployment类型的变量
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &instance.Spec.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": instance.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": instance.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  instance.Name,
							Image: instance.Spec.Image,
							Ports: []corev1.ContainerPort{ // 定义容器端口
								{
									ContainerPort: 80,
									Name:          "http",
								},
							},
						},
					},
				},
			},
		},
	}

	// Set AppService instance as the owner and controller
	_ = ctrl.SetControllerReference(instance, deploy, r.Scheme)

	return deploy
}

// 根据 CRD 中的声明去填充 Service 的内容
func (r *AppServiceReconciler) serviceForAppService(instance *appzhangsichencnv1.AppService) *corev1.Service {
	// 定义一个Service类型的变量
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-svc", // 为了避免名称冲突,这里自动为svc加上后缀
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": instance.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}

	// Set AppService instance as the owner and controller
	_ = ctrl.SetControllerReference(instance, service, r.Scheme)

	return service
}
