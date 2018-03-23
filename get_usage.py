from datetime import datetime
import os
import time
import xml.etree.ElementTree
import requests
from influxdb import InfluxDBClient

debug = os.environ.get('ABB_DEBUG', 0) == 1

db_name = os.environ.get('INFLUX_DB', 'aussiebb')
db = InfluxDBClient(
    os.environ.get('INFLUX_HOST', '127.0.0.1'),
    os.environ.get('INFLUX_PORT', 8086),
    os.environ.get('INFLUX_USER', 'root'),
    os.environ.get('INFLUX_PASS', 'root'),
    db_name)

def http_get(user, passw):
    URL = "https://my.aussiebroadband.com.au/usage.php?xml=yes"
    data = {
        'login_username': user,
        'login_password': passw
    }
    resp = requests.post(URL, data)
    tree = xml.etree.ElementTree.fromstring(resp.content)
    data = dict()

    for child in tree:
        data[child.tag] = child.text

    return {
        'download':    int(data['down1']),
        'upload':      int(data['up1']),
        'allowance':   int(data['allowance1_mb']) * 1000 * 1000,
        'left':        int(data['left1']),
        'lastupdated': datetime.strptime(data['lastupdated'], '%Y-%m-%d %H:%M:%S'),
        'rollover':    int(data['rollover'])
    }

if not db_name in db.get_list_database():
        db.create_database(db_name)

while True:
    users = os.environ['MYAUSSIE_USER'].split(',')
    passes = os.environ['MYAUSSIE_PASS'].split(',')
    json_body = []
    now = datetime.now()

    for i in range(len(users)):
        data = http_get(users[i], passes[i])
        json_body.append({
            'measurement': 'usage',
            'tags': {
                'user': users[i]
            },
            'time': now,
            'fields': {
                'download': data['download'],
                'upload': data['upload'],
                'allowance': data['allowance'],
                'left': data['left'],
                'rollover': data['rollover']
            }
        })

    if debug:
        print(json_body)
    db.write_points(json_body)
    time.sleep(os.environ.get('SLEEP_INTERVAL', 900))

