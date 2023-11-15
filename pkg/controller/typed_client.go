package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Object[T any] interface {
	*T
	client.Object
}

type TypedClient[T any, PT Object[T]] struct {
	Cli    client.Client
	Scheme *runtime.Scheme
}

type ObjList[T any, PT Object[T]] struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Items           []T `json:"items" protobuf:"bytes,2,rep,name=items"`
}

var _ runtime.Unstructured = &ObjList[corev1.Namespace, *corev1.Namespace]{}

func (t *ObjList[T, PT]) GetObjectKind() schema.ObjectKind {
	return t

}
func (t *ObjList[T, PT]) NewEmptyInstance() runtime.Unstructured {
	return &ObjList[T, PT]{}
}
func (t *ObjList[T, PT]) UnstructuredContent() map[string]interface{} {
	panic("implement me")
}
func (t *ObjList[T, PT]) SetUnstructuredContent(map[string]interface{}) {
	panic("implement me")
}
func (t *ObjList[T, PT]) IsList() bool {
	return true
}
func (t *ObjList[T, PT]) EachListItem(f func(runtime.Object) error) error {
	for _, item := range t.Items {
		var pitem PT = &item
		err := f(pitem)
		if err != nil {
			return err
		}
	}
	return nil
}
func (t *ObjList[T, PT]) EachListItemWithAlloc(f func(runtime.Object) error) error {
	for _, item := range t.Items {
		// shallow copy
		var item_copy T = item
		var pitem PT = &item_copy
		err := f(pitem)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *ObjList[T, PT]) DeepCopyObject() runtime.Object {
	ret := &ObjList[T, PT]{
		TypeMeta: t.TypeMeta,
		ListMeta: t.ListMeta,
		Items:    make([]T, len(t.Items)),
	}
	for i, item := range t.Items {
		var pitem PT = &item
		cloned := pitem.DeepCopyObject().(PT)
		ret.Items[i] = *cloned
	}
	return ret
}

func (t TypedClient[T, PT]) Get(ctx context.Context, key client.ObjectKey, opts ...client.GetOption) (*T, error) {
	var obj T
	var ptr PT = &obj
	err := t.Cli.Get(ctx, key, ptr, opts...)
	if err != nil {
		return nil, err
	}
	return &obj, nil
}

func (t TypedClient[T, PT]) List(ctx context.Context, opts ...client.ListOption) ([]T, error) {
	var objList ObjList[T, PT]
	var empty T
	var emptyPtr PT = &empty
	gvks, isUnversioned, err := t.Scheme.ObjectKinds(emptyPtr)

	if err != nil {
		return nil, err
	}
	if isUnversioned {
		return nil, fmt.Errorf("cannot create group-version-kind for unversioned type %T", emptyPtr)
	}
	if len(gvks) != 1 {
		return nil, fmt.Errorf("ambigous gvks  unversioned type %T %v", emptyPtr, gvks)
	}
	gvk := gvks[0]

	objList.TypeMeta.Kind = gvk.Kind + "List"
	objList.TypeMeta.APIVersion = gvk.GroupVersion().String()

	err = t.Cli.List(ctx, &objList, opts...)
	if err != nil {
		return nil, err
	}
	return objList.Items, nil
}

/*
use this to test:

package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/solo-io/gloo/v2/pkg/controller"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {

	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}

	mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
			fmt.Println("hello ns", req)
			return ctrl.Result{}, nil
		}))
	if err != nil {
		panic(err)
	}
	fmt.Println("hello world")
	go mgr.Start(context.TODO())
	mgr.GetCache().WaitForCacheSync(context.Background())
	cli := mgr.GetClient()

	gen := controller.TypedClient[corev1.Namespace, *corev1.Namespace]{cli, scheme}
	nl, err := gen.List(context.TODO())
	if err != nil {
		panic(err)
	}
	fmt.Println("hello nl", nl)
}



*/
