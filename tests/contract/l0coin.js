/***** 用合约来完成一个数字货币系统 *****/

// 合约创建时会被调用一次，之后就不会被调用
function L0Init(args) {
    console.log(args[0], args[1])
    var account = L0.Account();
    console.log(JSON.stringify({name:"Tim",number:12345}))
    L0.PutState("hello", JSON.stringify({name:"Tim",number:12345}))
    console.log(">>>:", L0.GetState("hello"))
    L0.PutState("minter", account.Address);
    L0.PutState("balances", {});
    // L0.PutState(args[0], JSON.stringify(args[1]))

// 合约账户余额
    var accountBalance = account.Balances

    for(var i in accountBalance){//用javascript的for/in循环遍历对象的属性 
        if (typeof(accountBalance[i])=="object"){
            for(var key in accountBalance[i]){
                console.log("amount key:",key,",amount value:",accountBalance[i][key]);
                }
            }else{
                console.log("key:",i," value:",accountBalance[i]);        
            }
    } 
    return true;
}

// 每次合约执行都调用
function L0Invoke(func, args) {
    var receiver = args[0];
    var amount = args[1];
    console.log("==>>>> ", L0.GetState("hello"))

    if ("mint" == func) {
        return mint(receiver, amount);
    } else if("send" == func) {
        return send(receiver, amount);
    } else if("transfer" == func) {
        return transfer(receiver, amount);
    }

    return false;
}

function L0Query(args) {
    console.log("call L0Query");
    return "query ok"
}

function mint(receiver, amount) {
    var sender = L0.Account().Address;
    var minter = L0.GetState("minter");
    var balances = L0.GetState("balances");

    if (minter != sender) {
        return false;
    }

    balances[receiver] = L0.toNumber(balancesMap[receiver], 0) + amount;
    L0.PutState("balances", balances);
    return true;
}

function send(receiver, amount) {
    var sender = L0.Account().Address;
    var balancesMap = L0.GetState("balances");

    var senderBalances = L0.toNumber(balancesMap[sender], 0);
    if (senderBalances < amount) {
        return false;
    }

    var recvBalances = L0.toNumber(balancesMap[receiver], 0);
    balances[sender] = senderBalances - amount;
    balances[receiver] = recvBalances + amount;

    L0.PutState("balances", balances);
    return true;
}

function transfer(receiver, amount) {
    console.log("call transfer...",receiver,amount);
    L0.Transfer(receiver,0,amount);
    return true;
}