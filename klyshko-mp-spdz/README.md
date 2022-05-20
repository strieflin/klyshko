# Klyshko Fake Correlated Randomness Generator

Provides a Klyshko Correlated Randomness Generator that uses the MP-SPDZ ability
to generate fake tuples.

## KII Tuple Type Mapping

The mapping from KII tuple types to the flags required for invoking the
`Fake-Offline.x` executable is as follows:

| Type                       | Flag           | Folder                | Header Length |
| -------------------------- | -------------- | --------------------- | ------------- |
| BIT_GFP                    | --nbits 0,n    | 2-p-128/Bits-p-P0     | 37            |
| BIT_GF2N                   | --nbits n,0    | 2-2-128/Bits-2-P0     | 34            |
| INPUT_MASK_GFP             | --ntriples 0,n | 2-p-128/Triples-p-P0  | 37            |
| INPUT_MASK_GF2N            | --ntriples n,0 | 2-2-128/Triples-2-P0  | 34            |
| INVERSE_TUPLE_GFP          | --ninverses n  | 2-p-128/Inverses-p-P0 | 37            |
| INVERSE_TUPLE_GF2N         | --ninverses n  | 2-2-128/Inverses-2-P0 | 34            |
| SQUARE_TUPLE_GFP           | --nsquares 0,n | 2-p-128/Squares-p-P0  | 37            |
| SQUARE_TUPLE_GF2N          | --nsquares n,0 | 2-2-128/Squares-2-P0  | 34            |
| MULTIPLICATION_TRIPLE_GFP  | --ntriples 0,n | 2-p-128/Triples-p-P0  | 37            |
| MULTIPLICATION_TRIPLE_GF2N | --ntriples n,0 | 2-2-128/Triples-2-P0  | 34            |