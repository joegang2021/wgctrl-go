//+build linux

package wglinux

import (
	"net"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"github.com/mdlayher/netlink/nltest"
	"github.com/mdlayher/wireguardctrl/internal/wglinux/internal/wgh"
	"github.com/mdlayher/wireguardctrl/internal/wgtest"
	"github.com/mdlayher/wireguardctrl/wgtypes"
	"golang.org/x/sys/unix"
)

func TestLinuxClientDevicesError(t *testing.T) {
	tests := []struct {
		name string
		msgs []genetlink.Message
	}{
		{
			name: "bad peer endpoint",
			msgs: []genetlink.Message{{
				Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
					Type: wgh.DeviceAPeers,
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
						Type: 0,
						Data: nltest.MustMarshalAttributes([]netlink.Attribute{
							{
								Type: wgh.PeerAEndpoint,
								Data: []byte{0xff},
							},
						}),
					}}),
				}}),
			}},
		},
		{
			name: "bad peer last handshake time",
			msgs: []genetlink.Message{{
				Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
					Type: wgh.DeviceAPeers,
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
						Type: 0,
						Data: nltest.MustMarshalAttributes([]netlink.Attribute{
							{
								Type: wgh.PeerALastHandshakeTime,
								Data: []byte{0xff},
							},
						}),
					}}),
				}}),
			}},
		},
		{
			name: "bad peer allowed IPs IP",
			msgs: []genetlink.Message{{
				Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
					Type: wgh.DeviceAPeers,
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
						Type: 0,
						Data: nltest.MustMarshalAttributes([]netlink.Attribute{
							{
								Type: wgh.PeerAAllowedips,
								Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
									Type: 0,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
										Type: wgh.AllowedipAIpaddr,
										Data: []byte{0xff},
									}}),
								}}),
							},
						}),
					}}),
				}}),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
				return tt.msgs, nil
			})
			defer c.Close()

			c.interfaces = func() ([]string, error) {
				return []string{okName}, nil
			}

			if _, err := c.Devices(); err == nil {
				t.Fatal("expected an error, but none occurred")
			}
		})
	}
}

func TestLinuxClientDevicesOK(t *testing.T) {
	const (
		testIndex = 2
		testName  = "wg1"
	)

	var (
		testKey wgtypes.Key
		keyA    = wgtest.MustPublicKey()
		keyB    = wgtest.MustPublicKey()
		keyC    = wgtest.MustPublicKey()
	)

	testKey[0] = 0xff

	tests := []struct {
		name       string
		interfaces func() ([]string, error)
		msgs       [][]genetlink.Message
		devices    []*wgtypes.Device
	}{
		{
			name: "basic",
			interfaces: func() ([]string, error) {
				return []string{okName, "wg1"}, nil
			},
			msgs: [][]genetlink.Message{
				{{
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{
						{
							Type: wgh.DeviceAIfindex,
							Data: nlenc.Uint32Bytes(okIndex),
						},
						{
							Type: wgh.DeviceAIfname,
							Data: nlenc.Bytes(okName),
						},
					}),
				}},
				{{
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{
						{
							Type: wgh.DeviceAIfindex,
							Data: nlenc.Uint32Bytes(testIndex),
						},
						{
							Type: wgh.DeviceAIfname,
							Data: nlenc.Bytes(testName),
						},
					}),
				}},
			},
			devices: []*wgtypes.Device{
				{
					Name: okName,
					Type: wgtypes.LinuxKernel,
				},
				{
					Name: "wg1",
					Type: wgtypes.LinuxKernel,
				},
			},
		},
		{
			name: "complete",
			msgs: [][]genetlink.Message{{{
				Data: nltest.MustMarshalAttributes([]netlink.Attribute{
					{
						Type: wgh.DeviceAIfindex,
						Data: nlenc.Uint32Bytes(okIndex),
					},
					{
						Type: wgh.DeviceAIfname,
						Data: nlenc.Bytes(okName),
					},
					{
						Type: wgh.DeviceAPrivateKey,
						Data: testKey[:],
					},
					{
						Type: wgh.DeviceAPublicKey,
						Data: testKey[:],
					},
					{
						Type: wgh.DeviceAListenPort,
						Data: nlenc.Uint16Bytes(5555),
					},
					{
						Type: wgh.DeviceAFwmark,
						Data: nlenc.Uint32Bytes(0xff),
					},
					{
						Type: wgh.DeviceAPeers,
						Data: nltest.MustMarshalAttributes([]netlink.Attribute{
							{
								Type: 0,
								Data: nltest.MustMarshalAttributes([]netlink.Attribute{
									{
										Type: wgh.PeerAPublicKey,
										Data: testKey[:],
									},
									{
										Type: wgh.PeerAPresharedKey,
										Data: testKey[:],
									},
									{
										Type: wgh.PeerAEndpoint,
										Data: (*(*[unix.SizeofSockaddrInet4]byte)(unsafe.Pointer(&unix.RawSockaddrInet4{
											Addr: [4]byte{192, 168, 1, 1},
											Port: sockaddrPort(1111),
										})))[:],
									},
									{
										Type: wgh.PeerAPersistentKeepaliveInterval,
										Data: nlenc.Uint16Bytes(10),
									},
									{
										Type: wgh.PeerALastHandshakeTime,
										Data: (*(*[sizeofTimespec64]byte)(unsafe.Pointer(&timespec64{
											Sec:  10,
											Nsec: 20,
										})))[:],
									},
									{
										Type: wgh.PeerARxBytes,
										Data: nlenc.Uint64Bytes(100),
									},
									{
										Type: wgh.PeerATxBytes,
										Data: nlenc.Uint64Bytes(200),
									},
									{
										Type: wgh.PeerAAllowedips,
										Data: mustAllowedIPs([]net.IPNet{
											wgtest.MustCIDR("192.168.1.10/32"),
											wgtest.MustCIDR("fd00::1/128"),
										}),
									},
									{
										Type: wgh.PeerAProtocolVersion,
										Data: nlenc.Uint32Bytes(1),
									},
								}),
							},
							// "dummy" peer with only some necessary fields.
							{
								Type: 1,
								Data: nltest.MustMarshalAttributes([]netlink.Attribute{
									{
										Type: wgh.PeerAPublicKey,
										Data: testKey[:],
									},
									{
										Type: wgh.PeerAEndpoint,
										Data: (*(*[unix.SizeofSockaddrInet6]byte)(unsafe.Pointer(&unix.RawSockaddrInet6{
											Addr: [16]byte{
												0xfe, 0x80, 0x00, 0x00,
												0x00, 0x00, 0x00, 0x00,
												0x00, 0x00, 0x00, 0x00,
												0x00, 0x00, 0x00, 0x01,
											},
											Port: sockaddrPort(2222),
										})))[:],
									},
									// Explicitly set last handshake time to
									// UNIX timestamp 0 to test zero-value
									// time.Time logic.
									//
									// In addition, we'll also test the timespec32
									// logic here, although we would never
									// expect WireGuard to return mixed size
									// values on the same platform.
									{
										Type: wgh.PeerALastHandshakeTime,
										Data: (*(*[sizeofTimespec32]byte)(unsafe.Pointer(&timespec32{
											Sec:  0,
											Nsec: 0,
										})))[:],
									},
								}),
							},
						}),
					},
				}),
			}}},
			devices: []*wgtypes.Device{
				{
					Name:         okName,
					Type:         wgtypes.LinuxKernel,
					PrivateKey:   testKey,
					PublicKey:    testKey,
					ListenPort:   5555,
					FirewallMark: 0xff,
					Peers: []wgtypes.Peer{
						{
							PublicKey:    testKey,
							PresharedKey: testKey,
							Endpoint: &net.UDPAddr{
								IP:   net.IPv4(192, 168, 1, 1),
								Port: 1111,
							},
							PersistentKeepaliveInterval: 10 * time.Second,
							LastHandshakeTime:           time.Unix(10, 20),
							ReceiveBytes:                100,
							TransmitBytes:               200,
							AllowedIPs: []net.IPNet{
								wgtest.MustCIDR("192.168.1.10/32"),
								wgtest.MustCIDR("fd00::1/128"),
							},
							ProtocolVersion: 1,
						},
						{
							PublicKey: testKey,
							Endpoint: &net.UDPAddr{
								IP:   net.ParseIP("fe80::1"),
								Port: 2222,
							},
						},
					},
				},
			},
		},
		{
			name: "merge devices",
			msgs: [][]genetlink.Message{{
				// The "target" device.
				{
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{
						{
							Type: wgh.DeviceAIfname,
							Data: nlenc.Bytes(okName),
						},
						{
							Type: wgh.DeviceAPrivateKey,
							Data: testKey[:],
						},
						{
							Type: wgh.DeviceAPeers,
							Data: nltest.MustMarshalAttributes([]netlink.Attribute{
								{
									Type: 0,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{
										{
											Type: wgh.PeerAPublicKey,
											Data: keyA[:],
										},
										{
											Type: wgh.PeerAAllowedips,
											Data: mustAllowedIPs([]net.IPNet{
												wgtest.MustCIDR("192.168.1.10/32"),
												wgtest.MustCIDR("192.168.1.11/32"),
											}),
										},
									}),
								},
							}),
						},
					}),
				},
				// Continuation of first peer list, new peer list.
				{
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{
						{
							Type: wgh.DeviceAPeers,
							Data: nltest.MustMarshalAttributes([]netlink.Attribute{
								{
									Type: 0,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{
										{
											Type: wgh.PeerAPublicKey,
											Data: keyA[:],
										},
										{
											Type: wgh.PeerAAllowedips,
											Data: mustAllowedIPs([]net.IPNet{
												wgtest.MustCIDR("fd00:dead:beef:dead::/64"),
												wgtest.MustCIDR("fd00:dead:beef:ffff::/64"),
											}),
										},
									}),
								},
								{
									Type: 1,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{
										{
											Type: wgh.PeerAPublicKey,
											Data: keyB[:],
										},
										{
											Type: wgh.PeerAAllowedips,
											Data: mustAllowedIPs([]net.IPNet{
												wgtest.MustCIDR("10.10.10.0/24"),
												wgtest.MustCIDR("10.10.11.0/24"),
											}),
										},
									}),
								},
							}),
						},
					}),
				},
				// Continuation of prevoius peer list, new peer list.
				{
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{
						{
							Type: wgh.DeviceAPeers,
							Data: nltest.MustMarshalAttributes([]netlink.Attribute{
								{
									Type: 0,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{
										{
											Type: wgh.PeerAPublicKey,
											Data: keyB[:],
										},
										{
											Type: wgh.PeerAAllowedips,
											Data: mustAllowedIPs([]net.IPNet{
												wgtest.MustCIDR("10.10.12.0/24"),
												wgtest.MustCIDR("10.10.13.0/24"),
											}),
										},
									}),
								},
								{
									Type: 1,
									Data: nltest.MustMarshalAttributes([]netlink.Attribute{
										{
											Type: wgh.PeerAPublicKey,
											Data: keyC[:],
										},
										{
											Type: wgh.PeerAAllowedips,
											Data: mustAllowedIPs([]net.IPNet{
												wgtest.MustCIDR("fd00:1234::/32"),
												wgtest.MustCIDR("fd00:4567::/32"),
											}),
										},
									}),
								},
							}),
						},
					}),
				},
			}},
			devices: []*wgtypes.Device{
				{
					Name:       okName,
					Type:       wgtypes.LinuxKernel,
					PrivateKey: testKey,
					Peers: []wgtypes.Peer{
						{
							PublicKey: keyA,
							AllowedIPs: []net.IPNet{
								wgtest.MustCIDR("192.168.1.10/32"),
								wgtest.MustCIDR("192.168.1.11/32"),
								wgtest.MustCIDR("fd00:dead:beef:dead::/64"),
								wgtest.MustCIDR("fd00:dead:beef:ffff::/64"),
							},
						},
						{
							PublicKey: keyB,
							AllowedIPs: []net.IPNet{
								wgtest.MustCIDR("10.10.10.0/24"),
								wgtest.MustCIDR("10.10.11.0/24"),
								wgtest.MustCIDR("10.10.12.0/24"),
								wgtest.MustCIDR("10.10.13.0/24"),
							},
						},
						{
							PublicKey: keyC,
							AllowedIPs: []net.IPNet{
								wgtest.MustCIDR("fd00:1234::/32"),
								wgtest.MustCIDR("fd00:4567::/32"),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				cmd   = wgh.CmdGetDevice
				flags = netlink.Request | netlink.Dump
			)

			// Advance through the test messages on subsequent calls.
			var i int
			fn := func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
				defer func() { i++ }()

				return tt.msgs[i], nil
			}

			c := testClient(t, genltest.CheckRequest(familyID, cmd, flags, fn))
			defer c.Close()

			// Replace interfaces if necessary.
			if tt.interfaces != nil {
				c.interfaces = tt.interfaces
			}

			devices, err := c.Devices()
			if err != nil {
				t.Fatalf("failed to get devices: %v", err)
			}

			if diff := cmp.Diff(tt.devices, devices); diff != "" {
				t.Fatalf("unexpected devices (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_timespec32MemoryLayout(t *testing.T) {
	// Assume unix.Timespec has 32-bit integers exclusively.
	if a := runtime.GOARCH; a != "386" {
		t.Skipf("skipping, architecture %q not handled in 32-bit only test", a)
	}

	// Verify unix.Timespec and timespec32 have an identical memory layout.
	uts := unix.Timespec{
		Sec:  1,
		Nsec: 2,
	}

	if diff := cmp.Diff(sizeofTimespec32, int(unsafe.Sizeof(unix.Timespec{}))); diff != "" {
		t.Fatalf("unexpected timespec size (-want +got):\n%s", diff)
	}

	ts := *(*timespec32)(unsafe.Pointer(&uts))

	if diff := cmp.Diff(uts.Sec, ts.Sec); diff != "" {
		t.Fatalf("unexpected timespec seconds (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(uts.Nsec, ts.Nsec); diff != "" {
		t.Fatalf("unexpected timespec nanoseconds (-want +got):\n%s", diff)
	}
}

func Test_timespec64MemoryLayout(t *testing.T) {
	// Assume unix.Timespec has 64-bit integers exclusively.
	if a := runtime.GOARCH; a != "amd64" {
		t.Skipf("skipping, architecture %q not handled in 64-bit only test", a)
	}

	// Verify unix.Timespec and timespec64 have an identical memory layout.
	uts := unix.Timespec{
		Sec:  1,
		Nsec: 2,
	}

	if diff := cmp.Diff(sizeofTimespec64, int(unsafe.Sizeof(unix.Timespec{}))); diff != "" {
		t.Fatalf("unexpected timespec size (-want +got):\n%s", diff)
	}

	ts := *(*timespec64)(unsafe.Pointer(&uts))

	if diff := cmp.Diff(uts.Sec, ts.Sec); diff != "" {
		t.Fatalf("unexpected timespec seconds (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(uts.Nsec, ts.Nsec); diff != "" {
		t.Fatalf("unexpected timespec nanoseconds (-want +got):\n%s", diff)
	}
}