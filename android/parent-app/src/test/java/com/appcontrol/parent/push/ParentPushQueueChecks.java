package com.appcontrol.parent.push;

/** Pure scheduling policy scenarios; Android JobScheduler/runtime behavior remains separate acceptance. */
public final class ParentPushQueueChecks {
    private static void require(boolean value) {
        if (!value) throw new AssertionError("Parent queue policy failed");
    }

    private static final class Queues {
        ParentPushQueueState state = new ParentPushQueueState(false);
        boolean regular, expedited;
        int enqueues;

        void request(boolean highPriority, boolean expeditedQuota) {
            state.request();
            ParentPushQueueState.Queue next = ParentPushQueueState.choose(highPriority, regular, expedited);
            if (next == ParentPushQueueState.Queue.EXPEDITED && !expeditedQuota)
                next = ParentPushQueueState.fallback(regular);
            if (next == ParentPushQueueState.Queue.REGULAR) { require(!regular); regular = true; enqueues++; }
            if (next == ParentPushQueueState.Queue.EXPEDITED) { require(!expedited); expedited = true; enqueues++; }
        }
    }

    public static void main(String[] args) {
        Queues burst = new Queues();
        for (int index = 0; index < 10_000; index++) burst.request(false, true);
        require(burst.regular && !burst.expedited && burst.enqueues == 1 && burst.state.dirty());
        for (int index = 0; index < 10_000; index++) burst.request(true, true);
        require(burst.regular && burst.expedited && burst.enqueues == 2);
        for (int index = 0; index < 10_000; index++) burst.request(false, true);
        require(burst.enqueues == 2);

        // A hint arriving during an authenticated fetch requires another pass, not another work item.
        require(burst.state.claim(true) && !burst.state.dirty());
        burst.request(true, true);
        require(burst.state.dirty() && burst.enqueues == 2);
        require(burst.state.claim(false) && !burst.state.dirty());
        require(!burst.state.claim(false));
        // A hint after the framework queue closes creates a fresh item instead of disappearing.
        burst.regular = false; burst.expedited = false;
        burst.request(true, true);
        require(burst.expedited && burst.enqueues == 3 && burst.state.dirty());

        Queues fallback = new Queues();
        fallback.request(true, false);
        require(fallback.regular && !fallback.expedited && fallback.enqueues == 1 && fallback.state.dirty());
        for (int index = 0; index < 10_000; index++) fallback.request(true, false);
        require(fallback.enqueues == 1 && fallback.state.dirty());
        // Fallback must not cancel or replace existing regular work when an expedited request is refused.
        require(ParentPushQueueState.fallback(true) == ParentPushQueueState.Queue.NONE);
        require(ParentPushQueueState.choose(false, false, true) == ParentPushQueueState.Queue.NONE);

        // Death after clearing dirty leaves a framework item that forces one authenticated retry.
        require(fallback.state.claim(true));
        ParentPushQueueState restarted = new ParentPushQueueState(fallback.state.dirty());
        require(!restarted.dirty() && restarted.claim(true));
        // Death before enqueue (or reboot losing nonpersisted items) retains the dirty bit for a new queue.
        restarted.request();
        ParentPushQueueState lostFrameworkQueue = new ParentPushQueueState(restarted.dirty());
        require(lostFrameworkQueue.dirty());
        require(ParentPushQueueState.choose(false, false, false) == ParentPushQueueState.Queue.REGULAR);
        require(lostFrameworkQueue.claim(false));

        // Logout resets durable state and cancels both queues; no pending request survives that reset.
        ParentPushQueueState signedOutRestart = new ParentPushQueueState(false);
        require(!signedOutRestart.dirty() && !signedOutRestart.claim(false));
        require(!ParentPushQueueState.canAttempt(0));
        for (int delivery = 1; delivery <= 5; delivery++) require(ParentPushQueueState.canAttempt(delivery));
        require(!ParentPushQueueState.canAttempt(6)); // Fifth-attempt process death must not start a sixth request.
        System.out.println("parent-push-queue-checks-passed: burst bounds, late hints, priority separation, quota fallback and restart/reset recovery");
    }
}
