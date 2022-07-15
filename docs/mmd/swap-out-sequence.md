```mermaid
sequenceDiagram
participant A as Alice
participant B as Bob

Note over A: Alice wants to swap out

A->>B: swap out requests
activate B
Note over B: calculate onchain fee
B->>A: swap out agreement response
deactivate B
A-->>B: pay fee invoice
activate B
Note over B: create opening tx
B->>A: tx opened message
deactivate B
alt claim with preimage
Note over A: await opening tx has N confirmations
A-->>B: pay claim invoice
Note over A: broadcast claim tx
else refund cooperatively
Note over A: paying invoice fails
A-->>B: send coop close message
Note over B: broadcast claim tx
else refund after csv passes
Note over B: broadcast claim tx
end
```