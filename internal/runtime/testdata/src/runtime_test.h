#ifndef RUNTIME_TEST_H
#define RUNTIME_TEST_H


#define INC(val) ((val) + 1)

#define SUCCESS 0

#define GET_HEIGHT_EXP 1001
#define GET_HEIGHT_ERR INC(SUCCESS)

#define GET_SENDER_ERR INC(GET_HEIGHT_ERR)

#define GET_AMOUNT_EXP 5001
#define GET_AMOUNT_ERR INC(GET_SENDER_ERR)

#define GET_ENTRY_HASH_ERR INC(GET_AMOUNT_ERR)

#define GET_TIMESTAMP_EXP 1575938086
#define GET_TIMESTAMP_ERR INC(GET_ENTRY_HASH_ERR)

#define GET_PRECISION_EXP 4
#define GET_PRECISION_ERR INC(GET_TIMESTAMP_ERR)

#define GET_ADDRESS_ERR INC(GET_PRECISION_ERR)

#endif // RUNTIME_TEST_H
