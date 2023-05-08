#!/usr/bin/python

from TraefikUpdater import TraefikUpdater


def main():
    updater = TraefikUpdater()
    updater.process()

    # This blocks
    updater.enter_update_loop()


if __name__ == "__main__":
    main()
