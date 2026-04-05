#!/usr/bin/env python3

import socket

SOCKET_PATH = "/tmp/voxi.sock"


def main() -> None:
    try:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.connect(SOCKET_PATH)
        sock.sendall(b"TOGGLE\n")
        sock.close()
    except FileNotFoundError:
        # Socket not found — daemon not running
        pass
    except Exception:
        # Any other error — silently ignore
        pass


if __name__ == "__main__":
    main()
