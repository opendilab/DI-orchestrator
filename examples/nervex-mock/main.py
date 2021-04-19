import time
import argparse
from threading import Thread

from worker import MockCoordinator, MockActor, MockLearner, MockLearnerAggregator

def info(args):
    print("====")

def start_cooridnator(args):
    host, port      = args.listen, args.port
    nervex_server   = args.server

    coordinator = MockCoordinator(host, port, nervex_server)
    def shutdown_monitor():
        while True:
            time.sleep(3)
            if coordinator.system_shutdown_flag:
                coordinator.close()
                break

    shutdown_monitor_thread = Thread(target=shutdown_monitor, args=(), daemon=True, name='shutdown_monitor')
    shutdown_monitor_thread.start()

    coordinator.start()

def start_actor(args):
    host, port = args.listen, args.port
    actor = MockActor(host, port)
    actor.start()

def start_learner(args):
    host, port = args.listen, args.port
    learner = MockLearner(host, port)
    learner.start()

def start_aggregator(args):
    slave_host, slave_port = args.slave_listen, args.slave_port
    master_host, master_port = args.master_listen, args.master_port
    aggregator = MockLearnerAggregator(slave_host, slave_port, master_host, master_port)
    aggregator.start()

def parse_args():
    parser = argparse.ArgumentParser(prog="mock_nervex", description="")
    parser.set_defaults(func=info)
    subparsers = parser.add_subparsers()

    # Coordinator
    parser_coordinator = subparsers.add_parser("coordinator", help="start a new coordiantor")
    parser_coordinator.add_argument("--server", type=str, default='http://nervex-server.nervex-system:8080', help="ip address")
    parser_coordinator.add_argument("-l", "--listen", default="localhost", help="Specify the IP address on which the server listens")
    parser_coordinator.add_argument("-p", "--port", type=int, default=8000, help="Specify the port on which the server listens")
    parser_coordinator.set_defaults(func=start_cooridnator)

    # Actor
    parser_actor = subparsers.add_parser("actor", help="start a new actor")
    parser_actor.add_argument("-l", "--listen", default="localhost", help="Specify the IP address on which the server listens")
    parser_actor.add_argument("-p", "--port", type=int, default=13339, help="Specify the port on which the server listens")
    parser_actor.set_defaults(func=start_actor)

    # Learner
    parser_learner = subparsers.add_parser("learner", help="start a new learner")
    parser_learner.add_argument("-l", "--listen", default="localhost", help="Specify the IP address on which the server listens",)
    parser_learner.add_argument("-p", "--port", type=int, default=12333, help="Specify the port on which the server listens")
    parser_learner.set_defaults(func=start_learner)

    # Aggregator
    parser_aggregator = subparsers.add_parser("aggregator", help="start a new aggregator")
    parser_aggregator.add_argument("-sl", "--slave_listen", default="localhost", help="Specify the IP address of aggregator slave on which the server listens",)
    parser_aggregator.add_argument("-sp", "--slave_port", type=int, default=12334, help="Specify the port of aggregator slave on which the server listens")
    parser_aggregator.add_argument("-ml", "--master_listen", default="localhost", help="Specify the IP address of aggregator master on which the server listens",)
    parser_aggregator.add_argument("-mp", "--master_port", type=int, default=12335, help="Specify the port of aggregator master on which the server listens")
    parser_aggregator.set_defaults(func=start_aggregator)

    args = parser.parse_args()
    args.func(args)

if __name__ == "__main__":
    parse_args()