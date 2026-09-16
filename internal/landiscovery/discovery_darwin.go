//go:build darwin && cgo

package landiscovery

/*
// DNS-SD symbols are provided by libSystem on macOS and iOS.
#include <dns_sd.h>
#include <arpa/inet.h>
#include <netinet/in.h>
#include <poll.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

typedef struct {
 DNSServiceRef ref;
 int error, ready, port, count;
 uint32_t iface;
 char host[1009];
 char ips[16][128];
} oc_dns;

static void oc_registered(DNSServiceRef r, DNSServiceFlags f, DNSServiceErrorType e,
 const char *n,const char *t,const char *d,void *context){
 ((oc_dns*)context)->error=e;
}
static void oc_resolved(DNSServiceRef r,DNSServiceFlags f,uint32_t i,DNSServiceErrorType e,
 const char *full,const char *host,uint16_t port,uint16_t len,const unsigned char *txt,void *context){
 oc_dns *s=context;s->error=e;s->ready=1;
 if(!e){s->port=ntohs(port);s->iface=i;snprintf(s->host,sizeof(s->host),"%s",host);}
}
static void oc_address(DNSServiceRef r,DNSServiceFlags f,uint32_t i,DNSServiceErrorType e,
 const char *host,const struct sockaddr *address,uint32_t ttl,void *context){
 oc_dns *s=context;
 if(e){if(e!=kDNSServiceErr_NoSuchRecord)s->error=e;return;}
 if(!(f & kDNSServiceFlagsAdd))return;
 if(s->count<16){
  char *dst=s->ips[s->count];
  if(address->sa_family==AF_INET){
   inet_ntop(AF_INET,&((const struct sockaddr_in*)address)->sin_addr,dst,128);
   s->count++;
  }else if(address->sa_family==AF_INET6){
   const struct sockaddr_in6 *v6=(const struct sockaddr_in6*)address;
   inet_ntop(AF_INET6,&v6->sin6_addr,dst,128);
   if(IN6_IS_ADDR_LINKLOCAL(&v6->sin6_addr)){
    size_t n=strlen(dst);snprintf(dst+n,128-n,"%%%u",i);
   }
   s->count++;
  }
 }
 if(!(f & kDNSServiceFlagsMoreComing))s->ready=1;
}
static oc_dns *oc_new(){return calloc(1,sizeof(oc_dns));}
static void oc_free(oc_dns *s){if(s->ref)DNSServiceRefDeallocate(s->ref);free(s);}
static int oc_register(oc_dns *s,const char *name,int port){
 return DNSServiceRegister(&s->ref,kDNSServiceFlagsNoAutoRename,0,name,"_onecatch._tcp","local.",NULL,htons(port),0,NULL,oc_registered,s);
}
static int oc_resolve(oc_dns *s,const char *name){
 return DNSServiceResolve(&s->ref,0,0,name,"_onecatch._tcp","local.",oc_resolved,s);
}
static int oc_addresses(oc_dns *s){
 DNSServiceRefDeallocate(s->ref);s->ref=NULL;s->ready=0;
 return DNSServiceGetAddrInfo(&s->ref,0,s->iface,kDNSServiceProtocol_IPv4|kDNSServiceProtocol_IPv6,s->host,oc_address,s);
}
static int oc_poll(oc_dns *s){
 struct pollfd fd={DNSServiceRefSockFD(s->ref),POLLIN,0};
 int result=poll(&fd,1,100);
 if(result>0){int e=DNSServiceProcessResult(s->ref);if(e)return e;}
 return s->error;
}
static const char *oc_ip(oc_dns *s,int i){return s->ips[i];}
*/
import "C"

import (
	"context"
	"fmt"
	"time"
	"unsafe"
)

// Use the OS Bonjour daemon on Apple platforms, not raw multicast sockets.
func Advertise(fingerprint string, port int) (func(), error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid discovery port")
	}
	instance, err := Instance(fingerprint)
	if err != nil {
		return nil, err
	}
	state := C.oc_new()
	if state == nil {
		return nil, fmt.Errorf("allocate Bonjour state")
	}
	name := C.CString(instance)
	defer C.free(unsafe.Pointer(name))
	if code := C.oc_register(state, name, C.int(port)); code != 0 {
		C.oc_free(state)
		return nil, fmt.Errorf("Bonjour register: %d", code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer C.oc_free(state)
		for ctx.Err() == nil {
			if C.oc_poll(state) != 0 {
				return
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}

func Resolve(ctx context.Context, fingerprint string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	instance, err := Instance(fingerprint)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	state := C.oc_new()
	if state == nil {
		return nil, fmt.Errorf("allocate Bonjour state")
	}
	defer C.oc_free(state)
	name := C.CString(instance)
	defer C.free(unsafe.Pointer(name))
	if code := C.oc_resolve(state, name); code != 0 {
		return nil, fmt.Errorf("Bonjour resolve: %d", code)
	}
	wait := func() error {
		for state.ready == 0 && ctx.Err() == nil {
			if code := C.oc_poll(state); code != 0 {
				return fmt.Errorf("Bonjour lookup: %d", code)
			}
		}
		return ctx.Err()
	}
	if err := wait(); err != nil {
		return nil, err
	}
	if code := C.oc_addresses(state); code != 0 {
		return nil, fmt.Errorf("Bonjour address: %d", code)
	}
	if err := wait(); err != nil {
		return nil, err
	}
	var urls []string
	// A and AAAA answers can arrive in separate callbacks. Collect both so
	// an unreachable IPv6 address cannot hide a working IPv4 endpoint.
	until := time.Now().Add(200 * time.Millisecond)
	for ctx.Err() == nil && time.Now().Before(until) {
		if C.oc_poll(state) != 0 {
			break
		}
	}
	for i := 0; i < int(state.count); i++ {
		if u := endpoint(C.GoString(C.oc_ip(state, C.int(i))), int(state.port)); u != "" {
			urls = append(urls, u)
		}
	}
	return urls, nil
}
