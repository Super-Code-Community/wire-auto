import os
import sys

sys.path.insert(0, os.path.join(os.environ["WIRE_SDK_DIR"], "python"))

from wire import Script


def main():
    s = Script()
    s.start()
    s.log("hello from python")
    s.done(0)


if __name__ == "__main__":
    main()
