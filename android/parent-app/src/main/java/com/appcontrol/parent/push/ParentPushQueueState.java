package com.appcontrol.parent.push;

/** One durable dirty bit coalesces hints; each stable framework queue has at most one work item. */
public final class ParentPushQueueState {
    public static final int MAX_DELIVERIES = 5;
    public enum Queue { NONE, REGULAR, EXPEDITED }
    private boolean dirty;

    public ParentPushQueueState(boolean dirty) { this.dirty = dirty; }
    public boolean dirty() { return dirty; }
    public boolean request() {
        if (dirty) return false;
        dirty = true;
        return true;
    }
    public boolean claim(boolean retainedWork) {
        if (!dirty && !retainedWork) return false;
        dirty = false;
        return true;
    }
    public static Queue choose(boolean highPriority, boolean regularQueued, boolean expeditedQueued) {
        if (expeditedQueued) return Queue.NONE;
        if (highPriority) return Queue.EXPEDITED;
        return regularQueued ? Queue.NONE : Queue.REGULAR;
    }
    public static Queue fallback(boolean regularQueued) {
        return regularQueued ? Queue.NONE : Queue.REGULAR;
    }
    public static boolean canAttempt(int deliveries) {
        return deliveries >= 1 && deliveries <= MAX_DELIVERIES;
    }
}
