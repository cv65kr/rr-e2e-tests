<?php

/**
 * @var Goridge\RelayInterface $relay
 */

use Spiral\Goridge;
use Spiral\RoadRunner;
use Spiral\Goridge\StreamRelay;
use Spiral\RoadRunner\Jobs\Consumer;

ini_set('display_errors', 'stderr');
require dirname(__DIR__) . "/vendor/autoload.php";

$rr = new RoadRunner\Worker(new StreamRelay(\STDIN, \STDOUT));
$consumer = new Consumer($rr);

while ($task = $consumer->waitTask()) {
    try {
        sleep(60);
        $task->complete();
    } catch (\Throwable $e) {
        $rr->error((string)$e);
    }
}
