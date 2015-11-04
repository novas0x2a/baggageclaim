package baggageclaim_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/concourse/baggageclaim"
	"github.com/concourse/baggageclaim/api"
	"github.com/concourse/baggageclaim/client"
	"github.com/concourse/baggageclaim/volume"
)

var _ = Describe("Baggage Claim Client", func() {
	Describe("getting the heartbeat interval from a TTL", func() {
		It("has an upper bound of 1 minute", func() {
			interval := client.IntervalForTTL(500 * time.Second)

			Expect(interval).To(Equal(time.Minute))
		})

		Context("when the TTL is small", func() {
			It("Returns an interval that is half of the TTL", func() {
				interval := client.IntervalForTTL(5 * time.Second)

				Expect(interval).To(Equal(2500 * time.Millisecond))
			})
		})
	})
	Describe("Interacting with the server", func() {
		var (
			bcServer *ghttp.Server
			logger   lager.Logger
			bcClient baggageclaim.Client
		)

		BeforeEach(func() {
			bcServer = ghttp.NewServer()
			logger = lagertest.NewTestLogger("client")
			bcClient = client.New(bcServer.URL())
		})

		AfterEach(func() {
			bcServer.Close()
		})

		Describe("Looking up a volume by handle", func() {
			It("heartbeats immediately to reset the TTL", func() {
				didHeartbeat := make(chan struct{})

				expectedVolume := volume.Volume{
					Handle:     "some-handle",
					Path:       "some-path",
					Properties: volume.Properties{},
					TTL:        volume.TTL(1),
					ExpiresAt:  time.Now().Add(time.Second),
				}

				bcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/volumes/some-handle"),
						ghttp.RespondWithJSONEncoded(http.StatusOK, expectedVolume),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("PUT", "/volumes/some-handle/ttl"),
						func(w http.ResponseWriter, r *http.Request) {
							close(didHeartbeat)
						},
						ghttp.RespondWith(http.StatusNoContent, ""),
					),
				)
				volume, found, err := bcClient.LookupVolume(logger, "some-handle")
				Expect(volume.Handle()).To(Equal(expectedVolume.Handle))
				Expect(found).To(BeTrue())
				Expect(err).ToNot(HaveOccurred())

				Eventually(didHeartbeat, time.Second).Should(BeClosed())
			})

			Context("when the volume's TTL is 0", func() {
				It("does not heartbeat", func() {
					bcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/volumes/some-handle"),
							ghttp.RespondWithJSONEncoded(200, volume.Volume{
								Handle:     "some-handle",
								Path:       "some-path",
								Properties: volume.Properties{},
								TTL:        volume.TTL(0),
								ExpiresAt:  time.Now().Add(time.Second),
							}),
						),
					)
					_, _, err := bcClient.LookupVolume(logger, "some-handle")
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(1) // wait to verify it is not heartbeating
				})
			})

			Context("when the intial heartbeat fails", func() {
				It("reports that the volume could not be found", func() {
					bcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/volumes/some-handle"),
							ghttp.RespondWithJSONEncoded(200, volume.Volume{
								Handle:     "some-handle",
								Path:       "some-path",
								Properties: volume.Properties{},
								TTL:        volume.TTL(1),
								ExpiresAt:  time.Now().Add(time.Second),
							}),
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("PUT", "/volumes/some-handle/ttl"),
							func(w http.ResponseWriter, r *http.Request) {
								api.RespondWithError(w, volume.ErrVolumeDoesNotExist, http.StatusNotFound)
							},
						),
					)
					foundVolume, found, err := bcClient.LookupVolume(logger, "some-handle")
					Expect(foundVolume).To(BeNil())
					Expect(found).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when the volume does not exist", func() {
				It("reports that the volume could not be found", func() {
					bcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/volumes/some-handle"),
							ghttp.RespondWith(http.StatusNotFound, ""),
						),
					)
					foundVolume, found, err := bcClient.LookupVolume(logger, "some-handle")
					Expect(foundVolume).To(BeNil())
					Expect(found).To(BeFalse())
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Describe("Listing volumes", func() {
			Context("when the inital heartbeat fails for a volume", func() {
				It("it is omitted from the returned list of volumes", func() {
					bcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/volumes"),
							ghttp.RespondWithJSONEncoded(200, []volume.Volume{
								{
									Handle:     "some-handle",
									Path:       "some-path",
									Properties: volume.Properties{},
									TTL:        volume.TTL(1),
									ExpiresAt:  time.Now().Add(time.Second),
								},
								{
									Handle:     "another-handle",
									Path:       "some-path",
									Properties: volume.Properties{},
									TTL:        volume.TTL(1),
									ExpiresAt:  time.Now().Add(time.Second),
								},
							}),
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("PUT", "/volumes/some-handle/ttl"),
							func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusNoContent)
							},
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("PUT", "/volumes/another-handle/ttl"),
							func(w http.ResponseWriter, r *http.Request) {
								api.RespondWithError(w, volume.ErrVolumeDoesNotExist, http.StatusNotFound)
							},
						),
					)
					volumes, err := bcClient.ListVolumes(logger, baggageclaim.VolumeProperties{})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(volumes)).To(Equal(1))
					Expect(volumes[0].Handle()).To(Equal("some-handle"))
				})
			})
		})
		Describe("Creating volumes", func() {
			Context("when the inital heartbeat fails for the volume", func() {
				It("reports that the volume could not be found", func() {
					bcServer.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("POST", "/volumes"),
							ghttp.RespondWithJSONEncoded(201, volume.Volume{
								Handle:     "some-handle",
								Path:       "some-path",
								Properties: volume.Properties{},
								TTL:        volume.TTL(1),
								ExpiresAt:  time.Now().Add(time.Second),
							}),
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("PUT", "/volumes/some-handle/ttl"),
							func(w http.ResponseWriter, r *http.Request) {
								api.RespondWithError(w, volume.ErrVolumeDoesNotExist, http.StatusNotFound)
							},
						),
					)
					createdVolume, err := bcClient.CreateVolume(logger, baggageclaim.VolumeSpec{})
					Expect(createdVolume).To(BeNil())
					Expect(err).To(Equal(volume.ErrVolumeDoesNotExist))
				})
			})
		})
	})
})