package controller

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetHostFromService(t *testing.T) {
	for _, tc := range []struct {
		name    string
		service *v1.Service
		want    string
		wantErr bool
	}{
		{
			name: "cluster_ip_service_ipv4",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-service",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.96.0.100",
				},
			},
			want: "10.96.0.100",
		},
		{
			name: "cluster_ip_service_ipv6",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-service",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "fd00:10:96::ae2c",
				},
			},
			want: "[fd00:10:96::ae2c]",
		},
		{
			name: "headless",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "headless-service",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "None",
				},
			},
			wantErr: true,
		},
		{
			name: "external_name",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "external-service",
					Namespace: "default",
				},
				Spec: v1.ServiceSpec{
					Type:         v1.ServiceTypeExternalName,
					ExternalName: "example.default.svc.cluster.local",
				},
			},
			want: "example.default.svc.cluster.local",
		},
		{
			name: "different_namespace_ipv4",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-service",
					Namespace: "production",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
				},
			},
			want: "10.0.0.1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getHostFromService(tc.service)

			if got != tc.want {
				t.Errorf("getHostFromService() = %q, want %q", got, tc.want)
			}

			if (err != nil) != tc.wantErr {
				t.Errorf("getHostFromService() returns unexpected error: %v", err)
			}
		})
	}
}
