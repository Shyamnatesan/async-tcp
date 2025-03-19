package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type Server struct {
	fd 					int 				  // socket file descriptor 
	epfd 				int				 	  // epoll instance file descriptor
	backlog				int					  // max_queue_length of pending connection
	sockAddr			syscall.SockaddrInet4 // socket address
	events 				[]syscall.EpollEvent  // to hold ready events
	active_connections	map[int]struct{}		  // active connections
}

func NewAsyncServer(port int, host string) *Server {

	ip := net.ParseIP(host).To4()
	if ip == nil {
		panic("Invalid IP address")
	}

	// create an epoll instance
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {

		panic(err)
	}

	// Create a socket
	sfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		panic(err)
	}

	// Set the socket to non-blocking mode
	err = syscall.SetNonblock(sfd, true)
	if err != nil {
		syscall.Close(sfd)
		panic(err)
	}

	// Create sockaddr structure
	socketAddr := &syscall.SockaddrInet4{
		Port: port,
		Addr: [4]byte{ip[0], ip[1], ip[2], ip[3]},
	}

	// Bind the socket
	err = syscall.Bind(sfd, socketAddr)
	if err != nil {
		syscall.Close(sfd)
		panic(err)
	}

	// register socket to the epoll instance
	socketEvent := unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd: int32(sfd),
	}

	err = unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, sfd, &socketEvent)
	if err != nil {
		syscall.Close(sfd)
		panic(err)
	}

	server := &Server{
		sockAddr: *socketAddr,
		fd: sfd,
		epfd: epfd,
		events: make([]syscall.EpollEvent, 10000),
		backlog: 1000,
		active_connections: make(map[int]struct{}),
	}

	return server
}


func (sr *Server) serve() {
	err := syscall.Listen(sr.fd, sr.backlog)
	if err != nil {
		syscall.Close(sr.fd)
		panic(err)
	}

	log.Println("server started on port ", sr.sockAddr.Port)

	// event loop starts 
	for {
		// wait for events
		num_ready_events, err := syscall.EpollWait(sr.epfd, sr.events, -1) 
		if err != nil {
			log.Println(err)
			continue
		}

		for i := 0; i < num_ready_events; i++ {
			ready_event_fd := int(sr.events[i].Fd)
			if ready_event_fd == sr.fd {
				/**
					This is a socket event. 
					1. accept the connection
					2. make the connection non blocking
					3. register the connection with our epoll instance
				**/
				cfd, _, err := syscall.Accept(ready_event_fd)
				if err != nil {
					log.Println(err)
					continue
				}

				err = syscall.SetNonblock(cfd, true)
				if err != nil {
					log.Println(err)
					sr.removeConnection(cfd)
					continue
				}

				client_event := syscall.EpollEvent{
					Events: syscall.EPOLLIN,
					Fd: int32(cfd),
				}

				err = syscall.EpollCtl(sr.epfd, syscall.EPOLL_CTL_ADD, cfd, &client_event)
				if err != nil {
					log.Println(err)
					sr.removeConnection(cfd)
					continue
				}

				sr.active_connections[cfd]  = struct{}{}


			}else{
				/**
					This is a connection event. 
					1. read and serve actual requests
				**/

				buffer := make([]byte, 1024)
				n, err := syscall.Read(ready_event_fd, buffer)

				if n == 0 {
					sr.removeConnection(ready_event_fd)
					continue
				}

				if err != nil {
					log.Println(err)
					sr.removeConnection(ready_event_fd)
					continue
				}


				request := buffer[:n]
				_, err = syscall.Write(ready_event_fd, request)
				if err != nil {
					log.Println(err)
					sr.removeConnection(ready_event_fd)
					continue
				}
			}
		}
	}
}


func (sr *Server) removeConnection(cfd int) {
	unix.EpollCtl(sr.epfd, unix.EPOLL_CTL_DEL, cfd, nil) 
	syscall.Close(cfd)
	delete(sr.active_connections, cfd)
}


func (sr *Server) shutdown() {
    fmt.Println("Shutting down server gracefully...")

    // Step 1: Close all active connections
    for cfd := range sr.active_connections {
        unix.EpollCtl(sr.epfd, unix.EPOLL_CTL_DEL, cfd, nil)

        if err := syscall.Close(cfd); err != nil {
            fmt.Printf("Error closing client connection (fd=%d): %v\n", cfd, err)
        }
    }

    // Step 2: Remove the listening socket from epoll
    if err := unix.EpollCtl(sr.epfd, unix.EPOLL_CTL_DEL, sr.fd, nil); err != nil {
        fmt.Printf("Error removing server socket from epoll: %v\n", err)
    }

    // Step 3: Close the server socket
    if err := syscall.Close(sr.fd); err != nil {
        fmt.Printf("Error closing server socket: %v\n", err)
    }

    // Step 4: Close the epoll instance
    if err := syscall.Close(sr.epfd); err != nil {
        fmt.Printf("Error closing epoll instance: %v\n", err)
    }

    fmt.Println("Shutdown complete.")
    os.Exit(0)
}
