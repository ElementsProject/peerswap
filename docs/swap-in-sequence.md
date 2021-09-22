```mermaid
sequenceDiagram
participant A as Alice
participant B as Bob

Note over A: Alice wants to swap in

A->>B: swap in request
activate B
Note over B: check channel balance
B->>A: swap in agreement response
deactivate B
activate A
Note over A: create openening tx
A->>B: tx opened message
deactivate A
alt claim with preimage
activate B
Note over B: await opening tx has N confirmations
B-->>A: pay claim invoic
Note over B: broadcast claim tx (preimage, pubkey bob)
deactivate B
else claim after cltv passes
Note over A: broadcast claim tx (pubkey alice)
end
```