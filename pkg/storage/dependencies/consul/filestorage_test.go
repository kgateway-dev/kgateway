package consul_test

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	. "github.com/solo-io/gloo/pkg/storage/dependencies/consul"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Client", func() {
	var rootPath string
	var consul *api.Client
	BeforeEach(func() {
		rootPath = RandString(4)
		c, err := api.NewClient(api.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())
		consul = c
	})
	AfterEach(func() {
		consul.KV().DeleteTree(rootPath, nil)
	})
	Describe("files", func() {
		Describe("create", func() {
			It("creates the file as a consul key", func() {
				client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				input := &dependencies.File{
					Ref:      "myfile",
					Contents: []byte("foo"),
				}
				fi, err := client.Create(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(fi).NotTo(Equal(input))
				p, _, err := consul.KV().Get(rootPath+"/files/"+input.Ref, nil)
				Expect(err).NotTo(HaveOccurred())
				fileFromConsul := &dependencies.File{
					Ref:             strings.TrimPrefix(p.Key, rootPath+"/files/"),
					Contents:        p.Value,
					ResourceVersion: fmt.Sprintf("%v", p.ModifyIndex),
				}
				Expect(fi).To(Equal(fileFromConsul))
			})
			It("creates binary files without any problem as a consul key", func() {
				contents := []byte{1, 2, 3, unicode.MaxASCII + 1}
				client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				input := &dependencies.File{
					Ref:      "myfile",
					Contents: contents,
				}
				fi, err := client.Create(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(fi).NotTo(Equal(input))
				p, _, err := consul.KV().Get(rootPath+"/files/"+input.Ref, nil)
				Expect(err).NotTo(HaveOccurred())
				fileFromConsul := &dependencies.File{
					Ref:             strings.TrimPrefix(p.Key, rootPath+"/files/"),
					Contents:        p.Value,
					ResourceVersion: fmt.Sprintf("%v", p.ModifyIndex),
				}
				Expect(fi).To(Equal(fileFromConsul))
				get, err := client.Get(input.Ref)
				Expect(err).NotTo(HaveOccurred())
				Expect(input.Contents).To(Equal(get.Contents))
			})
			It("errors when creating the same file twice", func() {
				client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				input := &dependencies.File{
					Ref:      "myfile",
					Contents: []byte("foo"),
				}
				_, err = client.Create(input)
				Expect(err).NotTo(HaveOccurred())
				_, err = client.Create(input)
				Expect(err).To(HaveOccurred())
			})
			Describe("update", func() {
				It("fails if the file doesn't exist", func() {
					client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
					Expect(err).NotTo(HaveOccurred())
					input := &dependencies.File{
						Ref:      "myfile",
						Contents: []byte("foo"),
					}
					fi, err := client.Update(input)
					Expect(err).To(HaveOccurred())
					Expect(fi).To(BeNil())
				})
				It("fails if the resourceversion is not up to date", func() {
					client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
					Expect(err).NotTo(HaveOccurred())
					input := &dependencies.File{
						Ref:      "myfile",
						Contents: []byte("foo"),
					}
					_, err = client.Create(input)
					Expect(err).NotTo(HaveOccurred())
					v, err := client.Update(input)
					Expect(v).To(BeNil())
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("resource version"))
				})
				It("updates the file", func() {
					client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
					Expect(err).NotTo(HaveOccurred())
					input := &dependencies.File{
						Ref:      "myfile",
						Contents: []byte("foo"),
					}
					fi, err := client.Create(input)
					Expect(err).NotTo(HaveOccurred())
					changed := &dependencies.File{
						Ref:             input.Ref,
						Contents:        []byte("bar"),
						ResourceVersion: fi.ResourceVersion,
					}
					out, err := client.Update(changed)
					Expect(err).NotTo(HaveOccurred())
					Expect(out.Contents).To(Equal(changed.Contents))
				})
				Describe("get", func() {
					It("fails if the file doesn't exist", func() {
						client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
						Expect(err).NotTo(HaveOccurred())
						fi, err := client.Get("foo")
						Expect(err).To(HaveOccurred())
						Expect(fi).To(BeNil())
					})
					It("returns the file", func() {
						client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
						Expect(err).NotTo(HaveOccurred())
						input := &dependencies.File{
							Ref:      "myfile",
							Contents: []byte("foo"),
						}
						fi, err := client.Create(input)
						Expect(err).NotTo(HaveOccurred())
						out, err := client.Get(input.Ref)
						Expect(err).NotTo(HaveOccurred())
						Expect(out).To(Equal(fi))
						input.ResourceVersion = out.ResourceVersion
						Expect(out).To(Equal(input))
					})
				})
				Describe("list", func() {
					It("returns all existing files", func() {
						client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
						Expect(err).NotTo(HaveOccurred())
						input1 := &dependencies.File{
							Ref:      "myfile1",
							Contents: []byte("foo"),
						}
						input2 := &dependencies.File{
							Ref:      "myfile2",
							Contents: []byte("foo"),
						}
						input3 := &dependencies.File{
							Ref:      "myfile3",
							Contents: []byte("foo"),
						}
						fi1, err := client.Create(input1)
						Expect(err).NotTo(HaveOccurred())
						fi2, err := client.Create(input2)
						Expect(err).NotTo(HaveOccurred())
						fi3, err := client.Create(input3)
						Expect(err).NotTo(HaveOccurred())
						out, err := client.List()
						Expect(err).NotTo(HaveOccurred())
						Expect(out).To(ContainElement(fi1))
						Expect(out).To(ContainElement(fi2))
						Expect(out).To(ContainElement(fi3))
					})
				})
				Describe("watch", func() {
					It("watches", func() {
						client, err := NewFileStorage(api.DefaultConfig(), rootPath, time.Minute)
						Expect(err).NotTo(HaveOccurred())
						lists := make(chan []*dependencies.File, 3)
						stop := make(chan struct{})
						defer close(stop)
						errs := make(chan error)
						w, err := client.Watch(&dependencies.FileEventHandlerFuncs{
							UpdateFunc: func(updatedList []*dependencies.File, _ *dependencies.File) {
								lists <- updatedList
							},
						})
						Expect(err).NotTo(HaveOccurred())
						go func() {
							w.Run(stop, errs)
						}()
						input1 := &dependencies.File{
							Ref:      "myfile1",
							Contents: []byte("foo"),
						}
						input2 := &dependencies.File{
							Ref:      "myfile2",
							Contents: []byte("foo"),
						}
						input3 := &dependencies.File{
							Ref:      "myfile3",
							Contents: []byte("foo"),
						}
						fi1, err := client.Create(input1)
						Expect(err).NotTo(HaveOccurred())
						fi2, err := client.Create(input2)
						Expect(err).NotTo(HaveOccurred())
						fi3, err := client.Create(input3)
						Expect(err).NotTo(HaveOccurred())

						var list []*dependencies.File
						Eventually(func() []*dependencies.File {
							select {
							default:
								return nil
							case l := <-lists:
								list = l
								return l
							}
						}).Should(HaveLen(3))
						Expect(list).To(HaveLen(3))
						Expect(list).To(ContainElement(fi1))
						Expect(list).To(ContainElement(fi2))
						Expect(list).To(ContainElement(fi3))
					})
				})
			})
		})
	})
})
