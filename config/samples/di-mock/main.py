import time
import argparse
from threading import Thread, Event
import logging

from worker import MockCoordinator, MockCollector, MockLearner, MockLearnerAggregator

def info(args):
    print("====")

def start_cooridnator(args):
    if args.disable_flask_log == True:
        log = logging.getLogger('werkzeug')
        log.disabled = True

    host, port      = args.listen, args.port
    di_server   = args.server

    coordinator = MockCoordinator(host, port, di_server)
    coordinator.start()
    system_shutdown_event = Event()

    def shutdown_monitor():
        while True:
            time.sleep(3)
            if coordinator.system_shutdown_flag:
                coordinator.close()
                system_shutdown_event.set()
                break

    shutdown_monitor_thread = Thread(target=shutdown_monitor, args=(), daemon=True, name='shutdown_monitor')
    shutdown_monitor_thread.start()
    system_shutdown_event.wait()
    print("[DI parallel pipeline]Your RL agent is converged, you can refer to 'log' and 'tensorboard' for details")

def start_collector(args):
    host, port = args.listen, args.port
    collector = MockCollector(host, port)
    collector.start()

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
    parser = argparse.ArgumentParser(prog="mock_di", description="")
    parser.set_defaults(func=info)
    subparsers = parser.add_subparsers()

    # Coordinator
    parser_coordinator = subparsers.add_parser("coordinator", help="start a new coordiantor")
    parser_coordinator.add_argument("--server", type=str, default='http://di-server.di-system:8080', help="ip address")
    parser_coordinator.add_argument("-l", "--listen", default="localhost", help="Specify the IP address on which the server listens")
    parser_coordinator.add_argument("-p", "--port", type=int, default=8000, help="Specify the port on which the server listens")
    parser_coordinator.add_argument('--disable_flask_log', default=True, type=bool, help='whether disable flask log')
    parser_coordinator.set_defaults(func=start_cooridnator)

    # Collector
    parser_collector = subparsers.add_parser("collector", help="start a new collector")
    parser_collector.add_argument("-l", "--listen", default="localhost", help="Specify the IP address on which the server listens")
    parser_collector.add_argument("-p", "--port", type=int, default=13339, help="Specify the port on which the server listens")
    parser_collector.set_defaults(func=start_collector)

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