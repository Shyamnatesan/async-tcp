package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)




func main() {

	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM) // Catch Ctrl + C (SIGINT)

	// epfd -> file descriptor of the epoll instance
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		fmt.Println("Error creating epoll instance", err)
		return 
	}


	// sfd -> file descriptor of the socket
	sfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		fmt.Println("Error creating a socket : ", err)
		return
	}

	socketAddr := syscall.SockaddrInet4{
		Port: 8080,
		Addr: [4]byte{127, 0, 0, 1},
	}

	syscall.Bind(sfd, &socketAddr)

	err = syscall.Listen(sfd, 10)
	if err != nil {
		fmt.Println("Error listening on 127.0.0.1:8080 : ", err)
		return
	}

	fmt.Println("server started on 127.0.0.1:8080 successfully ")



	event := unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd: int32(sfd),
	}

	err = unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, sfd, &event)
	if err != nil {
		fmt.Println("Error adding file descriptors, we want to watch to the epoll instance: ", err)
		return
	}

	fmt.Println("successfully created epoll for socket")

	go func() {
		<-sigChan
		fmt.Println("cleaning up")
		err = unix.EpollCtl(epfd, unix.EPOLL_CTL_DEL, sfd, nil)
		if err != nil {
			fmt.Println("Error deleting socket file descriptor, from our epoll instance : ", err)
		}
	
		syscall.Close(sfd)
		syscall.Close(epfd)
		os.Exit(0) // Ensure cleanup runs before exiting
	}()

	for {
		events := make([]unix.EpollEvent, 10)
		ready_events_count, err := unix.EpollWait(epfd, events, 60000)
		if err != nil {
			fmt.Println("Error waiting / receiving events from the epoll instance: ", err)
			return
		}

		fmt.Println("ready events count is ", ready_events_count)

		for i := 0; i < ready_events_count; i++ {
			if events[i].Fd == int32(sfd) {
				cfd, _, err := syscall.Accept(sfd)
				if err != nil {
					fmt.Println("Error accepting connection:", err)
					continue
				}

				event := unix.EpollEvent{
					Events: unix.EPOLLIN,
					Fd: int32(cfd),
				}
				err = unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, cfd, &event)
				if err != nil {
					fmt.Println("Error adding file descriptors, we want to watch to the epoll instance: ", err)
					syscall.Close(cfd)
					continue
				}
				fmt.Println("Client connected")
			}else {
				// Read data
				cfd := int(events[i].Fd)
				tempBuffer := make([]byte, 1024)
				readSize, err := syscall.Read(cfd, tempBuffer)
				if err != nil {
					fmt.Println("Error reading from client: , so closing the connection", err)
				}else{
					fmt.Println("Received from client:", string(tempBuffer[:readSize]))
				}
	
				err = unix.EpollCtl(epfd, unix.EPOLL_CTL_DEL, cfd, nil)
				if err != nil {
					fmt.Println("Error deleting connection file descriptor, from our epoll instance : ", err)
				}
				syscall.Close(cfd)
			}
		}
	}
}